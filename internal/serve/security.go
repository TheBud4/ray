package serve

import (
	"crypto/subtle"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// Cabeçalhos de segurança presentes em toda resposta, inclusive nas recusas.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; " +
	"connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

const (
	// tokenHeader carrega o token da sessão. Um cabeçalho próprio não viaja
	// sozinho numa requisição cross-site: o navegador exigiria preflight, e
	// este servidor nunca responde CORS.
	tokenHeader = "X-Ray-Token"
	apiPrefix   = "/api/"
	maxBodySize = 2 << 20
)

// staticTypes fixa o Content-Type de cada extensão servida, sem depender da
// tabela MIME do sistema operacional.
var staticTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".svg":  "image/svg+xml",
}

// staticRoutes é a lista fechada de caminhos públicos: só os que existem no
// embed entram. Qualquer outro caminho fora de /api/ é 404.
var staticRoutes = buildStaticRoutes(webFS, map[string]string{
	"/":            "web/index.html",
	"/index.html":  "web/index.html",
	"/app.js":      "web/app.js",
	"/app.css":     "web/app.css",
	"/favicon.svg": "web/favicon.svg",
})

// staticFile é um arquivo embutido pronto para servir.
type staticFile struct {
	body        []byte
	contentType string
}

func buildStaticRoutes(fsys fs.FS, candidates map[string]string) map[string]staticFile {
	routes := make(map[string]staticFile, len(candidates))
	for route, name := range candidates {
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			continue
		}
		ctype, ok := staticTypes[path.Ext(name)]
		if !ok {
			continue
		}
		routes[route] = staticFile{body: body, contentType: ctype}
	}
	return routes
}

// guard aplica as defesas, nesta ordem, a toda requisição — depois de pôr os
// cabeçalhos de segurança, que valem também para as recusas:
//
//  1. Host exatamente igual ao endereço do listener (403) — barra DNS
//     rebinding: uma página hostil que resolve o próprio domínio para
//     127.0.0.1 ainda manda o Host dela.
//  2. Método GET, PUT ou POST (405).
//  3. Rotas estáticas, sem token; outro caminho fora de /api/ é 404.
//  4. /api/*: token em X-Ray-Token, comparado em tempo constante (401).
//  5. Escrita em /api/*: Content-Type application/json e Origin igual a
//     http://<addr> (403) — barra CSRF de formulário e de outra origem.
//  6. Corpo de até 2 MiB (413), conferido pelo Content-Length e cortado pelo
//     MaxBytesReader.
//
// Em /api/* as recusas são JSON, no formato de apiError; nas rotas estáticas,
// texto puro.
type guard struct {
	token  string
	addr   string
	static map[string]staticFile
	api    http.Handler
}

func (g *guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isAPI := strings.HasPrefix(r.URL.Path, apiPrefix)
	setSecurityHeaders(w.Header(), isAPI)

	if g.addr == "" || r.Host != g.addr {
		refuse(w, isAPI, codeForbidden, "the Host header does not match this server")
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodPut, http.MethodPost:
	default:
		w.Header().Set("Allow", "GET, PUT, POST")
		refuse(w, isAPI, codeMethod, "allowed methods: GET, PUT, POST")
		return
	}
	if !isAPI {
		g.serveStatic(w, r)
		return
	}
	if !g.validToken(r.Header.Get(tokenHeader)) {
		writeCode(w, codeUnauthorized, "missing or wrong "+tokenHeader)
		return
	}
	if r.Method != http.MethodGet && (!isJSON(r.Header.Get("Content-Type")) || r.Header.Get("Origin") != "http://"+g.addr) {
		writeCode(w, codeForbidden, "a write needs Content-Type application/json and this server's Origin")
		return
	}
	if r.ContentLength > maxBodySize {
		writeCode(w, codePayloadTooLarge, tooLargeBody)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	g.api.ServeHTTP(w, r)
}

func setSecurityHeaders(h http.Header, isAPI bool) {
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	if isAPI {
		h.Set("Cache-Control", "no-store")
	}
}

// validToken compara em tempo constante. Um token configurado vazio recusa
// tudo, em vez de aceitar a requisição sem o cabeçalho.
func (g *guard) validToken(got string) bool {
	if g.token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(g.token)) == 1
}

// isJSON aceita o tipo application/json, sem diferenciar maiúsculas, com
// quaisquer parâmetros (charset ou outro).
func isJSON(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	return err == nil && media == "application/json"
}

// serveStatic serve só o que está em g.static, e só por GET.
func (g *guard) serveStatic(w http.ResponseWriter, r *http.Request) {
	f, ok := g.static[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", f.contentType)
	_, _ = w.Write(f.body)
}

// readBody lê o corpo inteiro; um corpo além do limite vira 413 e outra falha
// de leitura, 400. Devolve false quando já respondeu.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeBodyError(w, err)
		return nil, false
	}
	return body, true
}

// tooLargeBody é o detalhe da recusa de um corpo acima do limite.
const tooLargeBody = "the request body is larger than 2 MiB"

// writeBodyError responde uma falha ao ler ou decodificar o corpo: além do
// limite é 413; o resto, 400.
func writeBodyError(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeCode(w, codePayloadTooLarge, tooLargeBody)
		return
	}
	writeCode(w, codeBadRequest, err.Error())
}
