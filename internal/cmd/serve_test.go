package cmd

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TheBud4/ray/internal/catalog"
)

// fakeServe registra o que runServe fez, na ordem, sem rede nem navegador de verdade.
type fakeServe struct {
	mu        sync.Mutex
	events    []string
	url       string // a URL passada ao lançador
	opened    string // o argumento passado ao abrir o navegador
	handler   http.Handler
	cleaned   bool
	openErr   error
	listenErr error
	catErr    error
	openDone  chan struct{}
}

func (f *fakeServe) record(e string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
}

func (f *fakeServe) has(e string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.events {
		if x == e {
			return true
		}
	}
	return false
}

func (f *fakeServe) index(e string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.events {
		if x == e {
			return i
		}
	}
	return -1
}

func (f *fakeServe) deps(t *testing.T) serveDeps {
	t.Helper()
	f.openDone = make(chan struct{})
	return serveDeps{
		catalog: func() (catalog.Templates, error) {
			f.record("catalog")
			return catalog.Templates{Dir: t.TempDir()}, f.catErr
		},
		listen: func(port int) (net.Listener, error) {
			f.record("listen")
			if f.listenErr != nil {
				return nil, f.listenErr
			}
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err == nil {
				t.Cleanup(func() { ln.Close() })
			}
			return ln, err
		},
		launcher: func(url string) (string, func(), error) {
			f.record("launcher")
			f.url = url
			return "/tmp/launcher/open.html", func() { f.cleaned = true; f.record("cleanup") }, nil
		},
		open: func(path string) error {
			f.mu.Lock()
			f.opened = path
			f.mu.Unlock()
			f.record("open")
			close(f.openDone)
			return f.openErr
		},
		serve: func(ctx context.Context, ln net.Listener, h http.Handler) error {
			f.record("serve")
			f.handler = h
			<-ctx.Done()
			return nil
		},
	}
}

// run roda runServe até open ser chamado (ou, sem open, um instante), cancela e
// espera runServe voltar.
func (f *fakeServe) run(t *testing.T, port int, noOpen, dryRun bool, waitOpen bool) (out, errOut string, err error) {
	t.Helper()
	var o, e bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	deps := f.deps(t) // fora da goroutine: deps cria o canal que o teste espera
	go func() { done <- runServe(ctx, port, noOpen, dryRun, deps, &o, &e) }()
	if waitOpen {
		select {
		case <-f.openDone:
		case <-time.After(5 * time.Second):
			t.Fatal("the browser was never opened")
		}
	} else {
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runServe did not return after the context ended")
	}
	return o.String(), e.String(), err
}

var tokenInURL = regexp.MustCompile(`#token=([0-9a-f]{64})`)

func TestServeDryRunPrintsOneLineAndTouchesNothing(t *testing.T) {
	f := &fakeServe{}
	out, errOut, err := f.run(t, 0, false, true, false)
	if err != nil || out != "+ serve on 127.0.0.1\n" {
		t.Errorf("dry-run = %q, %v; want exactly %q", out, err, "+ serve on 127.0.0.1\n")
	}
	if len(f.events) != 0 {
		t.Errorf("dry-run did something: %v", f.events)
	}
	if strings.Contains(out+errOut, "token") {
		t.Errorf("dry-run printed a token: %q %q", out, errOut)
	}
}

func TestServeDefaultOpensALauncherFileNotTheTokenURL(t *testing.T) {
	f := &fakeServe{}
	out, errOut, err := f.run(t, 0, false, false, true)
	if err != nil {
		t.Fatal(err)
	}
	m := tokenInURL.FindStringSubmatch(f.url)
	if m == nil {
		t.Fatalf("the launcher URL %q carries no #token=<64 hex>", f.url)
	}
	if !strings.HasPrefix(f.url, "http://127.0.0.1:") {
		t.Errorf("the launcher URL %q must point at the loopback address", f.url)
	}
	if strings.Contains(f.opened, "token") || f.opened != "/tmp/launcher/open.html" {
		t.Errorf("the browser was opened with %q, want the launcher file path and no token", f.opened)
	}
	if strings.Contains(out+errOut, "token=") {
		t.Errorf("the token was printed although the launcher carries it: %q %q", out, errOut)
	}
	if !f.cleaned {
		t.Error("the launcher was not cleaned up when the server stopped")
	}
	// O token da URL é o que o handler exige.
	req := httptest.NewRequest("GET", "/api/ping", nil)
	req.Host = strings.TrimSuffix(strings.TrimPrefix(strings.SplitN(f.url, "/#", 2)[0], "http://"), "/")
	req.Header.Set("X-Ray-Token", m[1])
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("GET /api/ping with the URL's token and host = %d, want 200", rec.Code)
	}
}

func TestServeListensBeforeOpeningTheBrowser(t *testing.T) {
	f := &fakeServe{}
	if _, _, err := f.run(t, 0, false, false, true); err != nil {
		t.Fatal(err)
	}
	if !(f.index("listen") >= 0 && f.index("listen") < f.index("open")) {
		t.Errorf("events = %v, want listen before open", f.events)
	}
	if f.index("catalog") > f.index("listen") {
		t.Errorf("events = %v, want the catalog (and its baseline check) before listening", f.events)
	}
}

func TestServeNoOpenPrintsTheURLWithTheTokenAndOpensNothing(t *testing.T) {
	f := &fakeServe{}
	out, errOut, err := f.run(t, 0, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !tokenInURL.MatchString(out) {
		t.Errorf("--no-open output %q lacks the URL with #token=", out)
	}
	if f.has("open") || f.has("launcher") {
		t.Errorf("--no-open still opened a browser or wrote a launcher: %v", f.events)
	}
	if !strings.Contains(errOut, "⚠") {
		t.Errorf("--no-open stderr %q should warn that the URL carries the token", errOut)
	}
	assertNoEscape(t, out+errOut)
}

func TestServeSurvivesAFailingBrowser(t *testing.T) {
	f := &fakeServe{openErr: errors.New("no display")}
	_, errOut, err := f.run(t, 0, false, false, true)
	if err != nil {
		t.Errorf("a browser that fails to open must not fail the server: %v", err)
	}
	if !strings.Contains(errOut, "no display") || !strings.Contains(errOut, "⚠") {
		t.Errorf("stderr %q should warn about the browser failure", errOut)
	}
}

func TestServeRefusesBeforePrintingAnythingWhenThePortIsBusy(t *testing.T) {
	f := &fakeServe{listenErr: errors.New("address already in use")}
	out, errOut, err := f.run(t, 4000, false, false, false)
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error = %v, want the listen error", err)
	}
	if out != "" || strings.Contains(errOut, "http://") {
		t.Errorf("a failed listen printed output: %q %q", out, errOut)
	}
	if f.has("open") || f.has("launcher") || f.has("serve") {
		t.Errorf("a failed listen went on: %v", f.events)
	}
}

func TestServeRefusesToStartWithAnUnreadableBaseline(t *testing.T) {
	f := &fakeServe{catErr: errors.New("pristine.yaml: unreadable")}
	out, _, err := f.run(t, 0, false, false, false)
	if err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Errorf("error = %v, want the baseline error", err)
	}
	if out != "" || f.has("listen") {
		t.Errorf("an unreadable baseline still listened or printed: out=%q events=%v", out, f.events)
	}
}

func TestServeIsRegisteredWithItsFlags(t *testing.T) {
	c, _, err := newRootCmd().Find([]string{"serve"})
	if err != nil || c.Name() != "serve" {
		t.Fatalf("Find(serve) = %v, %v", c, err)
	}
	for _, flag := range []string{"port", "no-open"} {
		if c.Flags().Lookup(flag) == nil {
			t.Errorf("serve lacks --%s", flag)
		}
	}
}
