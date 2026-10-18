package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/catalog"
	"github.com/TheBud4/ray/internal/openutil"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/serve"
	"github.com/TheBud4/ray/internal/ui"
)

// openerWait é quanto o desligamento espera por quem abre o navegador.
const openerWait = 2 * time.Second

// serveDeps é tudo o que `ray serve` toca fora da memória: o catálogo de
// templates, a porta, o arquivo lançador, o navegador e o servidor. Os testes
// trocam cada um sem rede nem navegador de verdade.
type serveDeps struct {
	catalog  func() (catalog.Templates, error)
	listen   func(port int) (net.Listener, error)
	launcher func(url string) (string, func(), error)
	open     func(path string) error
	serve    func(ctx context.Context, ln net.Listener, h http.Handler) error
}

func newServeCmd() *cobra.Command {
	var (
		port   int
		noOpen bool
	)
	c := &cobra.Command{
		Use:   "serve",
		Short: "Serve a local page to view and edit the templates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			parent := cmd.Context()
			if parent == nil {
				parent = context.Background()
			}
			ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
			defer stop()
			deps := serveDeps{
				catalog:  func() (catalog.Templates, error) { return templateCatalog(true) },
				listen:   serve.Listen,
				launcher: serve.WriteLauncher,
				open:     func(path string) error { return openutil.Open(runner.ExecRunner{}, path) },
				serve:    serve.Serve,
			}
			return runServe(ctx, port, noOpen, flagDryRun, deps, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	c.Flags().IntVar(&port, "port", 0, "port on 127.0.0.1 to listen on (0 picks a free one)")
	c.Flags().BoolVar(&noOpen, "no-open", false, "print the URL with the session token instead of opening the browser")
	return c
}

// runServe sobe o editor até ctx terminar. A ordem importa: o catálogo (que
// confere a linha-base) antes da porta, e a porta antes do navegador — o
// navegador nunca é aberto para um endereço que ainda não escuta.
//
// O token nunca vai para o argv de outro processo: sem --no-open, quem abre o
// navegador recebe só o caminho de um arquivo privado que redireciona para a
// URL com o token. Com --no-open, a URL com o token vai para o terminal.
func runServe(ctx context.Context, port int, noOpen, dryRun bool, deps serveDeps, out, errOut io.Writer) error {
	if dryRun {
		fmt.Fprintln(out, "+ serve on 127.0.0.1")
		return nil
	}
	cat, err := deps.catalog()
	if err != nil {
		return err
	}
	ln, err := deps.listen(port)
	if err != nil {
		return err
	}
	token, err := serve.NewToken()
	if err != nil {
		ln.Close()
		return err
	}
	addr := ln.Addr().String()
	handler := serve.NewHandler(serve.Options{Templates: cat, Token: token}, addr)
	base := "http://" + addr
	url := base + "/#token=" + token

	p := ui.For(out)
	if noOpen {
		// A URL vai numa linha própria e sem cor: é para ser copiada inteira, e
		// o detalhe do título sai apagado no terminal.
		p.Title("ray serve", "")
		p.Line(url)
		p.Line("press Ctrl-C to stop")
		ui.For(errOut).Warning("this URL carries the session token",
			"anyone who can read this terminal can use it to edit your templates")
		return deps.serve(ctx, ln, handler)
	}

	path, cleanup, err := deps.launcher(url)
	if err != nil {
		ln.Close()
		return fmt.Errorf("writing the browser launcher: %w (use --no-open to print the URL instead)", err)
	}
	defer cleanup()

	p.Title("ray serve", base)
	p.Line("press Ctrl-C to stop")
	// Um navegador em sandbox (snap, flatpak) pode não enxergar o /tmp do
	// usuário e não abrir o lançador sem que o xdg-open acuse erro.
	p.Line("if the browser does not open, run ray serve --no-open")

	// O navegador abre em paralelo: o servidor já escuta, e um xdg-open lento
	// não atrasa a primeira requisição. Falhar ao abrir não derruba o servidor.
	opened := make(chan struct{})
	go func() {
		defer close(opened)
		if err := deps.open(path); err != nil {
			ui.For(errOut).Warning("could not open the browser for "+base+": "+err.Error(),
				"open "+path+" in a browser, or restart with --no-open to print the URL with the session token")
		}
	}()

	err = deps.serve(ctx, ln, handler)
	// Espera quem abre o navegador terminar, por um prazo: o aviso de falha não
	// se perde e o lançador só é apagado depois de entregue, mas um abridor
	// travado não segura o desligamento.
	select {
	case <-opened:
	case <-time.After(openerWait):
	}
	return err
}
