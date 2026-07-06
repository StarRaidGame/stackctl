package telemetry

import (
	"context"
	"net/http"
	"time"
)

// DispatcherStats mirrors the npc dispatcher's /stats JSON, plus OK: whether the
// last fetch succeeded (false → no dispatcher running; the pane shows "—").
type DispatcherStats struct {
	NpcsActive  int  `json:"npcs_active"`
	NpcsSpawned int  `json:"npcs_spawned"`
	OK          bool `json:"-"`
}

// Dispatcher fetches the live NPC count from the npc dispatcher's /stats endpoint.
type Dispatcher struct {
	url  string
	http *http.Client
}

// NewDispatcher targets the dispatcher /stats at addr (e.g. ":8091").
func NewDispatcher(addr string) *Dispatcher {
	return &Dispatcher{url: hostURL(addr), http: &http.Client{Timeout: 500 * time.Millisecond}}
}

// Fetch reads the NPC counts; OK=false (zero value) on any error.
func (d *Dispatcher) Fetch(ctx context.Context) DispatcherStats {
	var s DispatcherStats
	s.OK = getJSON(ctx, d.http, d.url, &s)
	if !s.OK {
		return DispatcherStats{}
	}
	return s
}
