package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TheBud4/ray/internal/catalog"
	"github.com/TheBud4/ray/internal/store"
	"github.com/TheBud4/ray/internal/testenv"
)

// hook é um template de hook embutido: o nome termina em .sh.tmpl.
const hook = "claude/hooks/guard-add.sh.tmpl"

// api monta o handler sobre um overlay real em t.TempDir(), com a linha-base
// num Store real: a corrida na linha-base é de verdade.
type api struct {
	h    http.Handler
	addr string
	dir  string
	opts *Options
}

func newAPI(t *testing.T, tweak func(*Options)) *api {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "templates")
	st := store.New(t.TempDir())
	o := &Options{
		Templates: catalog.Templates{
			Dir:         dir,
			Pristine:    func(rel string) (string, bool) { return st.PristineHash(dir, rel) },
			SetPristine: func(rel, hash string) error { return st.SetPristine(dir, rel, hash) },
			Verify:      st.Verify,
		},
		Token: "tok",
	}
	if tweak != nil {
		tweak(o)
	}
	return &api{h: NewHandler(*o, testAddr), addr: testAddr, dir: dir, opts: o}
}

// raw faz uma requisição autenticada e devolve o status e o corpo.
func (a *api) raw(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = a.addr
	req.Header.Set("X-Ray-Token", "tok")
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+a.addr)
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// call devolve o status e o objeto JSON da resposta.
func (a *api) call(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	code, b := a.raw(t, method, path, body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return code, out
}

// first devolve o nome do primeiro template que não é hook.
func (a *api) first(t *testing.T) string {
	t.Helper()
	entries, err := a.opts.Templates.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !catalog.IsHook(e.Name) {
			return e.Name
		}
	}
	t.Fatal("no editable template")
	return ""
}

func q(name string) string { return "?name=" + url.QueryEscape(name) }

// load lê o template pela API e devolve o objeto (com o hash da base).
func (a *api) load(t *testing.T, name string) map[string]any {
	t.Helper()
	code, info := a.call(t, "GET", "/api/template"+q(name), nil)
	if code != 200 {
		t.Fatalf("GET /api/template %q = %d %v", name, code, info)
	}
	return info
}

func (a *api) put(t *testing.T, name, content string, base any) (int, map[string]any) {
	t.Helper()
	return a.call(t, "PUT", "/api/template", map[string]any{"name": name, "content": content, "baseHash": base})
}

func TestListShowsStateAndReadOnlyAndNeverWritesToDisk(t *testing.T) {
	a := newAPI(t, nil)
	code, b := a.raw(t, "GET", "/api/templates", nil)
	var list []map[string]any
	if err := json.Unmarshal(b, &list); err != nil || code != 200 || len(list) == 0 {
		t.Fatalf("GET /api/templates = %d %s", code, b)
	}
	seenHook := false
	for _, e := range list {
		if e["name"] == hook {
			seenHook = true
			if e["readOnly"] != true {
				t.Errorf("hook entry = %v, want readOnly true", e)
			}
		}
		if e["state"] == nil || e["name"] == nil {
			t.Errorf("entry %v lacks name/state", e)
		}
	}
	if !seenHook {
		t.Errorf("the list lacks the hook %q", hook)
	}
	if _, err := os.Stat(a.dir); !os.IsNotExist(err) {
		t.Errorf("listing created the overlay directory: %v", err)
	}
}

func TestSaveThenReadRoundTrip(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	info := a.load(t, name)
	code, res := a.put(t, name, "# mine\n", info["hash"])
	if code != 200 || res["outcome"] != "edited" || res["state"] != "edited" {
		t.Fatalf("PUT = %d %v, want 200 edited", code, res)
	}
	again := a.load(t, name)
	if again["content"] != "# mine\n" || again["state"] != "edited" || again["hash"] != res["hash"] {
		t.Errorf("GET after save = %v, want the saved content, state edited and the hash Save returned", again)
	}
}

func TestAPIRejectsNamesOutsideTheList(t *testing.T) {
	a := newAPI(t, nil)
	for _, n := range []string{"../x.tmpl", "/etc/passwd", `..\x.tmpl`, "ünï.tmpl", "nope.tmpl", ""} {
		if code, _ := a.call(t, "GET", "/api/template"+q(n), nil); code != 404 {
			t.Errorf("GET %q = %d, want 404", n, code)
		}
		if code, _ := a.put(t, n, "x", ""); code != 404 {
			t.Errorf("PUT %q = %d, want 404", n, code)
		}
		if code, _ := a.call(t, "POST", "/api/preview", map[string]any{"name": n, "content": "x", "stack": false}); code != 404 {
			t.Errorf("preview %q = %d, want 404", n, code)
		}
		if code, _ := a.call(t, "POST", "/api/reset", map[string]any{"name": n, "force": true}); code != 404 {
			t.Errorf("reset %q = %d, want 404", n, code)
		}
		if code, _ := a.call(t, "GET", "/api/diff"+q(n), nil); code != 404 {
			t.Errorf("diff %q = %d, want 404", n, code)
		}
	}
	if _, err := os.Stat(a.dir); !os.IsNotExist(err) {
		t.Errorf("rejected names created the overlay directory: %v", err)
	}
}

func TestAPIMapsTypedErrors(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	cases := []struct {
		label   string
		name    string
		content string
		base    any
		want    int
		code    string
	}{
		{"hook", hook, "x", "", 403, "read-only-on-web"},
		{"stale", name, "x", "0000", 409, "stale"},
	}
	for _, c := range cases {
		code, res := a.put(t, c.name, c.content, c.base)
		if code != c.want || res["codigo"] != c.code || res["erro"] == nil || res["detalhe"] == nil {
			t.Errorf("%s = %d %v, want %d %q with erro and detalhe", c.label, code, res, c.want, c.code)
		}
	}
}

func TestAPIKeepsABrokenTemplateWith200(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	info := a.load(t, name)
	code, res := a.put(t, name, "ok\n{{.Stak}}\n", info["hash"])
	render, _ := res["render"].(map[string]any)
	if code != 200 || render == nil || render["line"] != float64(2) || render["message"] == "" {
		t.Fatalf("PUT = %d %v, want 200 with a render error on line 2", code, res)
	}
	if got, _ := os.ReadFile(filepath.Join(a.dir, name)); string(got) != "ok\n{{.Stak}}\n" {
		t.Errorf("a broken template must stay saved, file = %q", got)
	}
}

func TestAPIConcurrentSavesNeverLoseABaseline(t *testing.T) {
	a := newAPI(t, nil)
	entries, _ := a.opts.Templates.List()
	var names []string
	for _, e := range entries {
		if !catalog.IsHook(e.Name) && len(names) < 4 {
			names = append(names, e.Name)
		}
	}
	hashes := map[string]any{}
	for _, n := range names {
		hashes[n] = a.load(t, n)["hash"]
	}
	var wg sync.WaitGroup
	for _, n := range names {
		for range 2 { // dois salvamentos no mesmo template: um deles perde a corrida da base
			wg.Add(1)
			go func() {
				defer wg.Done()
				a.put(t, n, "# "+n+"\n", hashes[n])
			}()
		}
	}
	wg.Wait()
	after, _ := a.opts.Templates.List()
	for _, e := range after {
		for _, n := range names {
			if e.Name == n && e.NoBaseline {
				t.Errorf("%s lost its baseline to a concurrent save", e.Name)
			}
		}
	}
}

func TestAPIRejectsContentOverOneMiBDecoded(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	info := a.load(t, name)
	code, res := a.put(t, name, strings.Repeat("a", 1<<20+1), info["hash"])
	if code != 422 || res["codigo"] != "too-large" {
		t.Errorf("1 MiB + 1 = %d %v, want 422 too-large", code, res)
	}
	if code, res := a.call(t, "POST", "/api/preview", map[string]any{"name": name, "content": strings.Repeat("a", 1<<20+1), "stack": false}); code != 422 || res["codigo"] != "too-large" {
		t.Errorf("preview of 1 MiB + 1 = %d %v, want 422 too-large", code, res)
	}
	if code, _ := a.put(t, name, strings.Repeat("a", 1<<20), info["hash"]); code != 200 {
		t.Errorf("exactly 1 MiB = %d, want 200", code)
	}
}

func TestBadJSONIs400(t *testing.T) {
	a := newAPI(t, nil)
	req := httptest.NewRequest("PUT", "/api/template", strings.NewReader("{not json"))
	req.Host = a.addr
	req.Header.Set("X-Ray-Token", "tok")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+a.addr)
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("bad JSON = %d, want 400", rec.Code)
	}
}

func TestPreviewRendersADraftAndReportsErrorsWith200(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	code, res := a.call(t, "POST", "/api/preview", map[string]any{"name": name, "content": "# {{.ProjectName}}\n", "stack": false})
	if code != 200 || res["output"] != "# example\n" || res["err"] != nil {
		t.Fatalf("preview = %d %v, want output %q", code, res, "# example\n")
	}
	code, res = a.call(t, "POST", "/api/preview", map[string]any{"name": name, "content": "ok\n{{.Stak}}\n", "stack": false})
	e, _ := res["err"].(map[string]any)
	if code != 200 || e == nil || e["line"] != float64(2) {
		t.Errorf("preview of a broken draft = %d %v, want 200 with err on line 2", code, res)
	}
	if code, _ := a.call(t, "POST", "/api/preview", map[string]any{"name": hook, "content": "echo hi\n", "stack": false}); code != 200 {
		t.Errorf("preview of a hook = %d, want 200 (read-only on the web does not forbid previewing)", code)
	}
	if _, err := os.Stat(a.dir); !os.IsNotExist(err) {
		t.Errorf("preview created the overlay directory: %v", err)
	}
}

func TestPreviewIsSerialized(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	a := newAPI(t, func(o *Options) {
		o.previewHook = func() {
			once.Do(func() { close(entered) })
			<-release
		}
	})
	name := a.first(t)
	done := make(chan int)
	go func() {
		code, _ := a.call(t, "POST", "/api/preview", map[string]any{"name": name, "content": "x", "stack": false})
		done <- code
	}()
	<-entered
	if code, res := a.call(t, "POST", "/api/preview", map[string]any{"name": name, "content": "y", "stack": false}); code != 503 || res["codigo"] != "busy" {
		t.Errorf("second preview while one runs = %d %v, want 503 busy", code, res)
	}
	close(release)
	if code := <-done; code != 200 {
		t.Errorf("first preview = %d, want 200", code)
	}
}

func TestPreviewTimesOutAndReleasesTheGate(t *testing.T) {
	var calls atomic.Int32
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	a := newAPI(t, func(o *Options) {
		o.previewTimeout = 300 * time.Millisecond
		o.previewHook = func() {
			if calls.Add(1) == 1 {
				<-stuck // o primeiro render "trava" até o fim do teste
			}
		}
	})
	name := a.first(t)
	code, res := a.call(t, "POST", "/api/preview", map[string]any{"name": name, "content": "x", "stack": false})
	e, _ := res["err"].(map[string]any)
	if code != 200 || e == nil || !strings.Contains(e["message"].(string), "timed out") {
		t.Fatalf("stuck preview = %d %v, want 200 with a timed out error", code, res)
	}
	if code, res := a.call(t, "POST", "/api/preview", map[string]any{"name": name, "content": "# ok\n", "stack": false}); code != 200 || res["output"] != "# ok\n" {
		t.Errorf("preview after a timeout = %d %v, want the gate released", code, res)
	}
}

func TestResetOfAnEditedTemplateNeedsForce(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	info := a.load(t, name)
	a.put(t, name, "# mine\n", info["hash"])
	code, res := a.call(t, "POST", "/api/reset", map[string]any{"name": name, "force": false})
	if code != 409 || res["codigo"] != "edited" || !strings.Contains(res["detalhe"].(string), "lines differ") {
		t.Errorf("reset without force = %d %v, want 409 edited naming the lines", code, res)
	}
	if got, _ := os.ReadFile(filepath.Join(a.dir, name)); string(got) != "# mine\n" {
		t.Errorf("a refused reset must change nothing, file = %q", got)
	}
	if code, res := a.call(t, "POST", "/api/reset", map[string]any{"name": name, "force": true}); code != 200 || res["action"] != "restored" {
		t.Errorf("reset with force = %d %v, want 200 restored", code, res)
	}
	if code, res := a.call(t, "POST", "/api/reset", map[string]any{"name": name, "force": false}); code != 200 || res["action"] != "already the default" {
		t.Errorf("reset of a current template = %d %v, want 200 already the default", code, res)
	}
	if code, res := a.call(t, "POST", "/api/reset", map[string]any{"name": hook, "force": true}); code != 403 || res["codigo"] != "read-only-on-web" {
		t.Errorf("reset of a hook = %d %v, want 403 read-only-on-web", code, res)
	}
}

func TestResetOfACustomTemplateIs422(t *testing.T) {
	a := newAPI(t, nil)
	if err := os.MkdirAll(a.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.dir, "mine.tmpl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, res := a.call(t, "POST", "/api/reset", map[string]any{"name": "mine.tmpl", "force": true}); code != 422 || res["codigo"] != "custom" {
		t.Errorf("reset of a custom template = %d %v, want 422 custom", code, res)
	}
}

func TestDiffAnswersWithTheCatalogDecision(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	if code, res := a.call(t, "GET", "/api/diff"+q(name), nil); code != 200 || res["kind"] != "new" {
		t.Errorf("diff of a new template = %d %v, want kind new", code, res)
	}
	info := a.load(t, name)
	a.put(t, name, "# mine\n", info["hash"])
	code, res := a.call(t, "GET", "/api/diff"+q(name), nil)
	if code != 200 || res["kind"] != "unified" || !strings.Contains(res["text"].(string), "overlay/"+name) {
		t.Errorf("diff of an edited template = %d %v, want a unified diff", code, res)
	}
}

func TestHookIsReadableAsReadOnly(t *testing.T) {
	a := newAPI(t, nil)
	info := a.load(t, hook)
	if info["readOnly"] != true {
		t.Errorf("hook info = %v, want readOnly true", info)
	}
}

func TestUnknownAPIRoutesAre404WithJSONErrors(t *testing.T) {
	a := newAPI(t, nil)
	code, res := a.call(t, "GET", "/api/nope", nil)
	if code != 404 || res["codigo"] == nil {
		t.Errorf("unknown route = %d %v, want 404 with a JSON error body", code, res)
	}
}

// O JSON do navegador só carrega UTF-8 válido, então o risco real é outro:
// salvar de volta um arquivo do disco que não é UTF-8 perderia os bytes dele.
func TestRefusesToSaveOverAFileThatIsNotUTF8(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	if err := os.MkdirAll(filepath.Dir(filepath.Join(a.dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.dir, name)
	if err := os.WriteFile(path, []byte("ok \xff\xfe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info := a.load(t, name)
	if info["utf8"] != false {
		t.Fatalf("info = %v, want utf8 false for a file with invalid bytes", info)
	}
	code, res := a.put(t, name, "ok\n", info["hash"])
	if code != 422 || res["codigo"] != "not-utf8" {
		t.Errorf("PUT over a non-UTF-8 file = %d %v, want 422 not-utf8", code, res)
	}
	if got, _ := os.ReadFile(path); string(got) != "ok \xff\xfe\n" {
		t.Errorf("the original bytes must survive a refused save, file = %q", got)
	}
}

// Um U+FFFD escrito de propósito é conteúdo legítimo e passa.
func TestAReplacementCharacterTypedOnPurposeIsSaved(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	info := a.load(t, name)
	if code, res := a.put(t, name, "ok \uFFFD\n", info["hash"]); code != 200 {
		t.Errorf("PUT with U+FFFD = %d %v, want 200", code, res)
	}
}

// Linha-base ilegível: quem escreve responde 422 e nada é gravado.
func TestAnUnreadableBaselineRefusesTheWrite(t *testing.T) {
	a := newAPI(t, func(o *Options) {
		o.Templates.Verify = func() error { return errors.New("pristine.yaml: unreadable") }
	})
	name := a.first(t)
	// O hash vem direto do catálogo: o GET também recusa quando a linha-base está ilegível.
	info, err := a.opts.Templates.Info(name)
	if err != nil {
		t.Fatal(err)
	}
	code, res := a.put(t, name, "# mine\n", info.Hash)
	if code != 422 || res["codigo"] != "refused" || !strings.Contains(res["detalhe"].(string), "unreadable") {
		t.Errorf("PUT with an unreadable baseline = %d %v, want 422 refused naming the cause", code, res)
	}
	if _, err := os.Stat(filepath.Join(a.dir, name)); !os.IsNotExist(err) {
		t.Errorf("a refused save created the file: %v", err)
	}
}

func TestJSONWithUnknownFieldsOrTrailingValuesIs400(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	info := a.load(t, name)
	bodies := map[string]string{
		"unknown field": `{"name":"` + name + `","content":"x","baseHash":"` + info["hash"].(string) + `","admin":true}`,
		"two values":    `{"name":"` + name + `","content":"x","baseHash":"` + info["hash"].(string) + `"}{"name":"other"}`,
		"wrong type":    `{"name":"` + name + `","content":7,"baseHash":""}`,
	}
	for label, body := range bodies {
		req := httptest.NewRequest("PUT", "/api/template", strings.NewReader(body))
		req.Host = a.addr
		req.Header.Set("X-Ray-Token", "tok")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+a.addr)
		rec := httptest.NewRecorder()
		a.h.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Errorf("%s = %d, want 400", label, rec.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(a.dir, name)); !os.IsNotExist(err) {
		t.Errorf("a refused body created the file: %v", err)
	}
}

// Cancelar a requisição não pode liberar a trava: o render continua rodando, e
// um cliente que aborta cada preview teria N renders ao mesmo tempo.
func TestCancelledPreviewKeepsTheGateUntilTheRenderEnds(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	a := newAPI(t, func(o *Options) {
		o.previewTimeout = 2 * time.Second
		o.previewHook = func() {
			once.Do(func() { close(entered) })
			<-release
		}
	})
	name := a.first(t)
	body, _ := json.Marshal(map[string]any{"name": name, "content": "x", "stack": false})
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, "POST", "/api/preview", bytes.NewReader(body))
	req.Host = a.addr
	req.Header.Set("X-Ray-Token", "tok")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+a.addr)
	done := make(chan struct{})
	go func() {
		a.h.ServeHTTP(httptest.NewRecorder(), req)
		close(done)
	}()
	<-entered
	cancel()
	time.Sleep(100 * time.Millisecond) // o handler já viu o cancelamento
	if code, res := a.call(t, "POST", "/api/preview", map[string]any{"name": name, "content": "y", "stack": false}); code != 503 || res["codigo"] != "busy" {
		t.Errorf("preview after the first was cancelled mid-render = %d %v, want 503 busy (the render still runs)", code, res)
	}
	close(release)
	<-done
}

// Um template que não renderiza dentro do prazo não pode ser salvo: o Save
// renderiza o que grava, e um laço sem saída seguraria o mutex de todas as
// escritas. A recusa solta o mutex, e o Save seguinte funciona.
func TestSaveRefusesATemplateThatTakesTooLongToRender(t *testing.T) {
	var calls atomic.Int32
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	a := newAPI(t, func(o *Options) {
		o.previewTimeout = 300 * time.Millisecond
		o.previewHook = func() {
			if calls.Add(1) == 1 {
				<-stuck // o primeiro render "trava" até o fim do teste
			}
		}
	})
	name := a.first(t)
	info := a.load(t, name)
	code, res := a.put(t, name, "# slow\n", info["hash"])
	if code != 422 || res["codigo"] != "render-timeout" {
		t.Fatalf("PUT of a template that never renders = %d %v, want 422 render-timeout", code, res)
	}
	if _, err := os.Stat(filepath.Join(a.dir, name)); !os.IsNotExist(err) {
		t.Errorf("a refused save created the file: %v", err)
	}
	if code, res := a.put(t, name, "# fast\n", info["hash"]); code != 200 {
		t.Errorf("PUT after the refusal = %d %v, want 200 (the write lock was released)", code, res)
	}
}

func TestAMethodNotAllowedOnAKnownRouteIsJSON405(t *testing.T) {
	a := newAPI(t, nil)
	code, res := a.call(t, "POST", "/api/templates", map[string]any{})
	if code != 405 || res["codigo"] != "method-not-allowed" {
		t.Errorf("POST /api/templates = %d %v, want 405 method-not-allowed in JSON", code, res)
	}
}

// Linha-base ilegível depois de o servidor subir: nada decide estado nem
// escreve — o Reset e as três leituras recusam com 422, como o CLI.
func TestAnUnreadableBaselineRefusesResetAndReadsToo(t *testing.T) {
	a := newAPI(t, func(o *Options) {
		o.Templates.Verify = func() error { return errors.New("pristine.yaml: unreadable") }
	})
	name := a.first(t)
	for _, c := range []struct {
		label, method, path string
		body                any
	}{
		{"list", "GET", "/api/templates", nil},
		{"info", "GET", "/api/template" + q(name), nil},
		{"diff", "GET", "/api/diff" + q(name), nil},
		{"reset", "POST", "/api/reset", map[string]any{"name": name, "force": true}},
	} {
		code, res := a.call(t, c.method, c.path, c.body)
		if code == 200 || res["codigo"] != "refused" || !strings.Contains(fmt.Sprint(res["detalhe"]), "unreadable") {
			t.Errorf("%s with an unreadable baseline = %d %v, want 422 refused naming the cause", c.label, code, res)
		}
	}
	if _, err := os.Stat(filepath.Join(a.dir, name)); !os.IsNotExist(err) {
		t.Errorf("a refused reset created the file: %v", err)
	}
}

// Duas abas salvam a partir da mesma base: só uma vence, a outra leva 409 e o
// arquivo é exatamente o da vencedora.
func TestTwoSavesFromTheSameBaseOnlyOneWins(t *testing.T) {
	a := newAPI(t, nil)
	entries, _ := a.opts.Templates.List()
	n := 0
	for _, e := range entries {
		if catalog.IsHook(e.Name) || n == 4 {
			continue
		}
		n++
		name := e.Name
		hash := a.load(t, name)["hash"]
		var wg sync.WaitGroup
		codes := make([]int, 2)
		for i := range codes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i], _ = a.put(t, name, fmt.Sprintf("# aba %d\n", i), hash)
			}()
		}
		wg.Wait()
		wins, stale := 0, 0
		for _, c := range codes {
			switch c {
			case 200:
				wins++
			case 409:
				stale++
			}
		}
		if wins != 1 || stale != 1 {
			t.Errorf("%s: statuses %v, want exactly one 200 and one 409", name, codes)
			continue
		}
		winner := 0
		if codes[1] == 200 {
			winner = 1
		}
		if got, _ := os.ReadFile(filepath.Join(a.dir, name)); string(got) != fmt.Sprintf("# aba %d\n", winner) {
			t.Errorf("%s: file = %q, want the winner's content", name, got)
		}
	}
}

// Um template que é symlink é recusado com 422 e o alvo fica como está.
func TestASymlinkedTemplateIs422AndTheTargetIsUntouched(t *testing.T) {
	a := newAPI(t, nil)
	name := a.first(t)
	outside := filepath.Join(t.TempDir(), "outside.tmpl")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(a.dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(a.dir, name)); err != nil {
		testenv.SymlinkUnavailable(t, err)
		t.Fatal(err)
	}
	info := a.load(t, name)
	code, res := a.put(t, name, "# mine\n", info["hash"])
	if code != 422 || res["codigo"] != "refused" || !strings.Contains(fmt.Sprint(res["detalhe"]), "symlink") {
		t.Errorf("PUT over a symlink = %d %v, want 422 refused naming the symlink", code, res)
	}
	if got, _ := os.ReadFile(outside); string(got) != "x" {
		t.Errorf("the symlink target was written: %q", got)
	}
}

// Um hook é recusado (403) antes de qualquer render: o PUT não paga os dois
// renders com prazo nem segura o mutex por causa de conteúdo lento.
func TestAHookPutIsRefusedBeforeAnyRender(t *testing.T) {
	var calls atomic.Int32
	a := newAPI(t, func(o *Options) {
		o.previewHook = func() { calls.Add(1) }
	})
	code, res := a.put(t, hook, "echo hi\n", "")
	if code != 403 || res["codigo"] != "read-only-on-web" {
		t.Errorf("PUT of a hook = %d %v, want 403 read-only-on-web", code, res)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("a refused hook PUT rendered %d times, want 0", n)
	}
}
