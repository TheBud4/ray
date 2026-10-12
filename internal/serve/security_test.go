package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testAddr = "127.0.0.1:52411"

func newTestHandler(t *testing.T) (h http.Handler, addr, token string) {
	t.Helper()
	return NewHandler(Options{Token: "tok"}, testAddr), testAddr, "tok"
}

// do faz uma requisição já com Host e token certos, para o teste mexer só no
// que quer provar.
func do(h http.Handler, method, path, addr, token string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = addr
	if token != "" {
		req.Header.Set("X-Ray-Token", token)
	}
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHostMustBeExactlyTheListener(t *testing.T) {
	h, addr, tok := newTestHandler(t)
	for _, host := range []string{"localhost:52411", "[::1]:52411", "127.0.0.1:52411.", "127.0.0.1:9999", "", "evil.com", "127.0.0.1.evil.com:52411"} {
		for _, path := range []string{"/api/ping", "/"} {
			if rec := do(h, "GET", path, host, tok, nil); rec.Code != 403 {
				t.Errorf("Host %q on %s = %d, want 403", host, path, rec.Code)
			}
		}
	}
	if rec := do(h, "GET", "/api/ping", addr, tok, nil); rec.Code != 200 {
		t.Errorf("exact Host = %d, want 200", rec.Code)
	}
}

func TestAPIRoutesNeedTheTokenButStaticFilesDoNot(t *testing.T) {
	h, addr, tok := newTestHandler(t)
	if c := do(h, "GET", "/api/ping", addr, "", nil).Code; c != 401 {
		t.Errorf("no token = %d, want 401", c)
	}
	if c := do(h, "GET", "/api/ping", addr, "wrong", nil).Code; c != 401 {
		t.Errorf("wrong token = %d, want 401", c)
	}
	if c := do(h, "GET", "/api/ping", addr, tok+"x", nil).Code; c != 401 {
		t.Errorf("token with a suffix = %d, want 401", c)
	}
	if c := do(h, "GET", "/api/ping", addr, tok, nil).Code; c != 200 {
		t.Errorf("right token = %d, want 200", c)
	}
	rec := do(h, "GET", "/", addr, "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("static / = %d %q, want 200 text/html without a token", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestWritesNeedJSONAndOurOrigin(t *testing.T) {
	h, addr, tok := newTestHandler(t)
	cases := []struct {
		name, ctype, origin string
		want                int
	}{
		{"no origin", "application/json", "", 403},
		{"foreign origin", "application/json", "http://evil.com", 403},
		{"same host other port", "application/json", "http://127.0.0.1:9999", 403},
		{"form content type", "application/x-www-form-urlencoded", "http://" + addr, 403},
		{"text/plain", "text/plain", "http://" + addr, 403},
		{"no content type", "", "http://" + addr, 403},
		{"ok", "application/json", "http://" + addr, 200},
		{"ok with charset", "application/json; charset=utf-8", "http://" + addr, 200},
	}
	for _, c := range cases {
		for _, method := range []string{"POST", "PUT"} {
			req := httptest.NewRequest(method, "/api/ping", strings.NewReader("{}"))
			req.Host = addr
			req.Header.Set("X-Ray-Token", tok)
			if c.ctype != "" {
				req.Header.Set("Content-Type", c.ctype)
			}
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("%s %s = %d, want %d", method, c.name, rec.Code, c.want)
			}
		}
	}
}

func TestMethodsOutsideGetPutPostAre405(t *testing.T) {
	h, addr, tok := newTestHandler(t)
	for _, m := range []string{"DELETE", "PATCH", "OPTIONS", "HEAD", "TRACE"} {
		if rec := do(h, m, "/api/ping", addr, tok, nil); rec.Code != 405 {
			t.Errorf("%s = %d, want 405", m, rec.Code)
		}
	}
}

func TestEveryResponseCarriesTheSecurityHeadersAndNoCookie(t *testing.T) {
	h, addr, tok := newTestHandler(t)
	for _, path := range []string{"/", "/api/ping", "/missing", "/api/missing"} {
		rec := do(h, "GET", path, addr, tok, nil)
		hd := rec.Header()
		csp := hd.Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'none'", "script-src 'self'", "style-src 'self'", "connect-src 'self'", "base-uri 'none'", "form-action 'none'", "frame-ancestors 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q lacks %q", path, csp, want)
			}
		}
		if hd.Get("X-Content-Type-Options") != "nosniff" || hd.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: missing nosniff/no-referrer: %v", path, hd)
		}
		if hd.Get("Set-Cookie") != "" {
			t.Errorf("%s: must set no cookie: %v", path, hd)
		}
		for k := range hd {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Errorf("%s: must set no CORS header, got %s", path, k)
			}
		}
	}
	if cc := do(h, "GET", "/api/ping", addr, tok, nil).Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("/api/ping Cache-Control = %q, want no-store", cc)
	}
	// Mesmo as recusas (403/401/405) levam os cabeçalhos.
	if rec := do(h, "GET", "/api/ping", "evil.com", tok, nil); rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("a 403 response lacks the security headers: %v", rec.Header())
	}
	if rec := do(h, "GET", "/api/ping", addr, "", nil); rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("a 401 response lacks the security headers: %v", rec.Header())
	}
}

func TestBodyOver2MiBIs413(t *testing.T) {
	h, addr, tok := newTestHandler(t)
	for _, size := range []int{2<<20 + 1, 3 << 20} {
		req := httptest.NewRequest("POST", "/api/ping", strings.NewReader(strings.Repeat("a", size)))
		req.Host = addr
		req.Header.Set("X-Ray-Token", tok)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+addr)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 413 {
			t.Errorf("body of %d bytes = %d, want 413", size, rec.Code)
		}
	}
}

func TestBodyAtTheLimitIsAccepted(t *testing.T) {
	h, addr, tok := newTestHandler(t)
	req := httptest.NewRequest("POST", "/api/ping", strings.NewReader(strings.Repeat("a", 2<<20)))
	req.Host = addr
	req.Header.Set("X-Ray-Token", tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+addr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("body of exactly 2 MiB = %d, want 200", rec.Code)
	}
}

func TestGETNeverChangesState(t *testing.T) {
	// O esqueleto só tem /api/ping; qualquer rota de GET que exista não pode
	// exigir Origin (é leitura) nem aceitar corpo como comando.
	h, addr, tok := newTestHandler(t)
	if rec := do(h, "GET", "/api/ping", addr, tok, nil); rec.Code != 200 {
		t.Errorf("GET without Origin = %d, want 200 (reads need no Origin)", rec.Code)
	}
}
