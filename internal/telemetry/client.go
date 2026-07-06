// Package telemetry polls the game server's read-only /stats endpoint (served on
// its AdminAddr — see server/internal/stats) for the live game numbers the TUI's
// telemetry pane shows: objects, sessions, tick rate.
package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Stats mirrors the server's stats.Snapshot, plus OK: whether the last fetch
// succeeded (false → the server is down or unreachable; the pane shows "—").
type Stats struct {
	Objects  int     `json:"objects"`
	Sessions int64   `json:"sessions"`
	TickHz   float64 `json:"tick_hz"`
	UptimeS  float64 `json:"uptime_s"`
	OK       bool    `json:"-"`
}

// Client fetches stats from one server's control surface.
type Client struct {
	url  string
	http *http.Client
}

// NewClient targets the server stats endpoint at addr (e.g. ":8080" or
// "host:8080"); a leading-colon address is treated as localhost.
func NewClient(addr string) *Client {
	return &Client{url: hostURL(addr), http: &http.Client{Timeout: 500 * time.Millisecond}}
}

// Fetch reads the current stats. On any error it returns a zero Stats with
// OK=false rather than failing — the UI keeps ticking when the server is down.
func (c *Client) Fetch(ctx context.Context) Stats {
	var s Stats
	s.OK = getJSON(ctx, c.http, c.url, &s)
	if !s.OK {
		return Stats{}
	}
	return s
}

// getJSON does a GET and decodes the body into dst, reporting success. Any
// error/non-200 → false (the caller shows "—"); it never panics or blocks past
// the client timeout.
func getJSON(ctx context.Context, c *http.Client, url string, dst any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(dst) == nil
}

// hostURL turns a ":port" or "host:port" address into an http /stats URL.
func hostURL(addr string) string {
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	return "http://" + host + "/stats"
}
