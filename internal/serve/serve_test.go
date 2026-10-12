package serve

import (
	"context"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"
)

func TestListenIsLoopbackOnlyAndRefusesABusyPort(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	if host != "127.0.0.1" {
		t.Errorf("listening on %q, want 127.0.0.1", host)
	}
	if p, _ := strconv.Atoi(port); p == 0 {
		t.Error("Listen(0) did not choose a free port")
	}
	p, _ := strconv.Atoi(port)
	if _, err := Listen(p); err == nil {
		t.Error("Listen on a busy port succeeded; want an error")
	}
	for _, bad := range []int{-1, 70000} {
		if _, err := Listen(bad); err == nil {
			t.Errorf("Listen(%d) succeeded; want an error", bad)
		}
	}
}

func TestNewTokenIsRandomHex(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(a) {
		t.Errorf("token %q is not 64 hex characters (32 bytes)", a)
	}
	if a == b {
		t.Error("two tokens are equal; each session needs its own")
	}
}

func TestServerHasTheFourTimeouts(t *testing.T) {
	s := newServer(http.NewServeMux())
	if s.ReadHeaderTimeout != 5*time.Second || s.ReadTimeout != 15*time.Second ||
		s.WriteTimeout != 30*time.Second || s.IdleTimeout != 60*time.Second {
		t.Errorf("timeouts = %v/%v/%v/%v, want 5s/15s/30s/60s",
			s.ReadHeaderTimeout, s.ReadTimeout, s.WriteTimeout, s.IdleTimeout)
	}
}

func TestServeShutsDownWhenTheContextEnds(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, http.NewServeMux()) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v after a clean shutdown, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after the context ended")
	}
}

func TestServeAnswersOverRealTCPOnLoopback(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, ln, NewHandler(Options{Token: "tok"}, addr))

	req, _ := http.NewRequest("GET", "http://"+addr+"/api/ping", nil)
	req.Header.Set("X-Ray-Token", "tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("GET /api/ping over TCP = %d, want 200", resp.StatusCode)
	}
}
