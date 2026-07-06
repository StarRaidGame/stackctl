package main

import "github.com/StarRaidGame/stackctl/internal/supervisor"

// services is the StarRaid stack, in start order (postgres first, then the server
// it backs, then the rest). Each Dir is a sibling submodule relative to the stack
// root; each command is that component's own `just` recipe, so stackctl stays thin
// and the components keep owning how they build/run.
//
// Optional services are excluded from "start all" and brought up on demand: the
// Godot client is a GUI window, and the reference bot needs connection flags/creds
// (dev-stub auth — set STARRAID_DEV_SECRET on the server).
func services() []supervisor.Service {
	return []supervisor.Service{
		{Name: "postgres", Dir: "server", Kind: supervisor.Oneshot, Run: "just db-up", Stop: "just db-down"},
		{Name: "server", Dir: "server", Kind: supervisor.Daemon, Run: "just run"},
		{Name: "admin", Dir: "admin", Kind: supervisor.Daemon, Run: "just run"},
		{Name: "dispatcher", Dir: "npc", Kind: supervisor.Daemon, Run: "just run-dispatcher"},
		{Name: "client", Dir: "client", Kind: supervisor.Daemon, Run: "just build && just run", Optional: true},
		{Name: "npc-bot", Dir: "npc", Kind: supervisor.Daemon, Run: "just run", Optional: true},
	}
}
