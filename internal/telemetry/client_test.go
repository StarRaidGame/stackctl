package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchParsesServerShape(t *testing.T) {
	// Mirrors server/internal/stats.Snapshot's JSON exactly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"objects":42,"sessions":3,"tick_hz":10,"uptime_s":12.5}`))
	}))
	defer srv.Close()

	c := &Client{url: srv.URL, http: srv.Client()}
	s := c.Fetch(context.Background())
	if !s.OK {
		t.Fatal("Fetch: OK=false for a healthy server")
	}
	if s.Objects != 42 || s.Sessions != 3 || s.TickHz != 10 {
		t.Fatalf("decoded stats = %+v", s)
	}
}

func TestFetchDownIsNotOK(t *testing.T) {
	c := NewClient("127.0.0.1:1") // nothing listening there
	if s := c.Fetch(context.Background()); s.OK {
		t.Fatal("Fetch: want OK=false when the server is unreachable")
	}
}

func TestNewClientNormalisesColonAddr(t *testing.T) {
	if got := NewClient(":8080").url; got != "http://localhost:8080/stats" {
		t.Fatalf("url = %q", got)
	}
}
