// Package serve é o servidor HTTP local do editor de templates: escuta só em
// 127.0.0.1, serve uma página estática embutida e uma API em /api/* protegida
// por token. Toda requisição passa pelas defesas de security.go antes de
// chegar a uma rota — o navegador do usuário também carrega páginas hostis,
// e elas podem tentar alcançar este servidor.
package serve

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/TheBud4/ray/internal/catalog"
)

// webFS guarda a página estática servida sem token.
//
//go:embed web
var webFS embed.FS

// Options é o que o handler precisa: os templates que a API expõe e o token
// que toda chamada a /api/* tem de apresentar.
type Options struct {
	Templates catalog.Templates
	Token     string

	// previewHook, se não for nil, roda na goroutine do preview logo antes do
	// render: é o ponto em que os testes seguram um render em andamento.
	previewHook func()
	// previewTimeout é o prazo de um preview; 0 é defaultPreviewTimeout.
	previewTimeout time.Duration
}

// Os quatro prazos do servidor e o prazo do desligamento.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// Listen abre o listener em 127.0.0.1; port 0 escolhe uma porta livre. Nunca
// escuta em outro endereço.
func Listen(port int) (net.Listener, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d: want 0-65535", port)
	}
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

// NewToken devolve 32 bytes aleatórios em hex minúsculo.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// NewHandler monta o handler completo. addr é o endereço real do listener
// ("127.0.0.1:52411"): o Host de toda requisição tem de ser exatamente ele.
func NewHandler(o Options, addr string) http.Handler {
	a := newAPIHandler(o)
	api := http.NewServeMux()
	api.HandleFunc("/api/ping", handlePing)
	api.HandleFunc("/api/templates", methods{http.MethodGet: a.list}.serve)
	api.HandleFunc("/api/template", methods{http.MethodGet: a.info, http.MethodPut: a.save}.serve)
	api.HandleFunc("/api/preview", methods{http.MethodPost: a.preview}.serve)
	api.HandleFunc("/api/diff", methods{http.MethodGet: a.diff}.serve)
	api.HandleFunc("/api/reset", methods{http.MethodPost: a.reset}.serve)
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeCode(w, codeNotFound, "no such API route")
	})
	return &guard{token: o.Token, addr: addr, static: staticRoutes, api: api}
}

// newServer embrulha h num http.Server com os quatro prazos: um cliente lento
// não segura uma conexão para sempre.
func newServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// Serve atende ln com h até ctx terminar e então desliga com prazo de 5 s.
// Um desligamento limpo devolve nil. O listener é fechado ao sair.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := newServer(h)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	select {
	case err := <-served:
		return ignoreClosed(err)
	case <-ctx.Done():
	}

	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		// O prazo venceu com conexões ainda abertas: corta todas.
		_ = srv.Close()
		<-served
		return fmt.Errorf("shutting down: %w", err)
	}
	return ignoreClosed(<-served)
}

// ignoreClosed traduz o erro de servidor desligado de propósito para nil.
func ignoreClosed(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// handlePing responde {"ok":true}; numa escrita, lê o corpo inteiro antes,
// para o limite de tamanho valer de fato.
func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		if _, ok := readBody(w, r); !ok {
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}` + "\n"))
}
