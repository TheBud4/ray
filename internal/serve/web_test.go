package serve

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func mustRead(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(webFS, name)
	if err != nil {
		t.Fatalf("the embedded page lacks %s: %v", name, err)
	}
	return string(b)
}

// Os três arquivos da página têm de estar no embed; um nome errado viraria 404
// em silêncio.
func TestTheEmbeddedPageHasItsThreeFilesAndServesThem(t *testing.T) {
	h, addr, _ := newTestHandler(t)
	want := map[string]string{
		"/":            "text/html",
		"/index.html":  "text/html",
		"/app.js":      "text/javascript",
		"/app.css":     "text/css",
		"/favicon.svg": "image/svg+xml",
	}
	for path, ctype := range want {
		rec := do(h, "GET", path, addr, "", nil)
		if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), ctype) {
			t.Errorf("GET %s = %d %q, want 200 %s without a token", path, rec.Code, rec.Header().Get("Content-Type"), ctype)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s has an empty body", path)
		}
	}
}

// Todo dado entra no DOM por textContent/createElement: o que um repositório
// hostil escreve (nome de servidor, erro de perfil) nunca pode virar HTML.
func TestEmbeddedJSNeverUsesDangerousSinks(t *testing.T) {
	js := mustRead(t, "web/app.js")
	for _, bad := range []string{
		"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function",
		"srcdoc", "javascript:", "setAttribute(\"on", "setAttribute('on", "setAttribute(\"style", "setAttribute('style", "cssText",
	} {
		if strings.Contains(js, bad) {
			t.Errorf("app.js uses %q; data may only enter the DOM through textContent/createElement", bad)
		}
	}
}

// A CSP não tem 'unsafe-inline': a página não pode depender de script, estilo
// nem manipulador de evento inline.
func TestIndexHasNoInlineScriptStyleOrHandler(t *testing.T) {
	html := mustRead(t, "web/index.html")
	for _, tag := range regexp.MustCompile(`(?is)<script\b[^>]*>`).FindAllString(html, -1) {
		if !strings.Contains(strings.ToLower(tag), "src=") {
			t.Errorf("inline <script> in index.html: %s", tag)
		}
	}
	if regexp.MustCompile(`(?is)<script\b[^>]*>\s*[^<\s]`).MatchString(html) {
		t.Error("a <script> in index.html has inline content")
	}
	if regexp.MustCompile(`(?i)<style\b`).MatchString(html) || regexp.MustCompile(`(?i)\sstyle\s*=`).MatchString(html) {
		t.Error("index.html has inline style")
	}
	if regexp.MustCompile(`(?i)\son[a-z]+\s*=`).MatchString(html) {
		t.Error("index.html has an inline on* handler")
	}
	for _, want := range []string{`src="/app.js"`, `href="/app.css"`, `rel="icon"`, `href="/favicon.svg"`, "<title>", `name="viewport"`, "<html lang="} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
}

func TestEmbeddedFilesReferenceNoOtherHost(t *testing.T) {
	absolute := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]|https?://`)
	for _, name := range []string{"web/index.html", "web/app.css", "web/app.js"} {
		src := mustRead(t, name)
		if m := absolute.FindString(src); m != "" {
			t.Errorf("%s references an absolute URL (%q); the page may only talk to its own server", name, m)
		}
		if strings.Contains(src, "@import") {
			t.Errorf("%s uses @import; nothing may be fetched besides /app.js and /app.css", name)
		}
	}
}

// O token sai do fragmento da URL na hora (o histórico não o guarda) e vive só
// na sessionStorage; vai em todo pedido para a API no cabeçalho certo.
func TestThePageDropsTheTokenFromTheURLAndSendsItInTheHeader(t *testing.T) {
	js := mustRead(t, "web/app.js")
	for _, want := range []string{"history.replaceState", "sessionStorage", "X-Ray-Token", "location.hash"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
	if strings.Contains(js, "localStorage") {
		t.Error("app.js must not keep the token in localStorage (it would outlive the session)")
	}
}

// A página fala com todas as rotas da API e respeita o contrato de gravação.
func TestThePageUsesTheWholeAPIAndTheSaveContract(t *testing.T) {
	js := mustRead(t, "web/app.js")
	for _, want := range []string{
		"/api/templates", "/api/template", "/api/preview", "/api/diff", "/api/reset",
		"baseHash", "readOnly", "stale", "render-timeout",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
}

// Os estados e as cores são os do CLI; o CSS os define e segue o tema do sistema.
func TestTheStylesheetColorsTheStatesAndFollowsTheSystemTheme(t *testing.T) {
	css := mustRead(t, "web/app.css")
	for _, state := range []string{"outdated", "new", "current", "edited", "custom"} {
		if !strings.Contains(css, state) {
			t.Errorf("app.css has no rule for the %s state", state)
		}
	}
	if !strings.Contains(css, "prefers-color-scheme") {
		t.Error("app.css does not follow the system color scheme")
	}
}

// O favicon é um SVG autônomo: o namespace xmlns é a única URL permitida, e nada
// de script, link, imagem ou folha de estilo externos dentro dele.
func TestTheFaviconIsASelfContainedSVG(t *testing.T) {
	svg := mustRead(t, "web/favicon.svg")
	if !strings.Contains(svg, "<svg") {
		t.Fatal("favicon.svg is not an SVG document")
	}
	for _, bad := range []string{"<script", "<foreignObject", "<image", "<use", "<style", "xlink:href", " href=", "onload", "onerror", "@import", "url("} {
		if strings.Contains(strings.ToLower(svg), strings.ToLower(bad)) {
			t.Errorf("favicon.svg contains %q", bad)
		}
	}
	rest := strings.ReplaceAll(svg, "http://www.w3.org/2000/svg", "")
	if strings.Contains(rest, "http://") || strings.Contains(rest, "https://") {
		t.Error("favicon.svg references an URL besides the SVG namespace")
	}
}
