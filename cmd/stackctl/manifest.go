package main

import "github.com/StarRaidGame/stackctl/internal/supervisor"

// services is the StarRaid stack, in start order (postgres first, then the server
// it backs, then the rest). Each Dir is a sibling submodule relative to the stack
// root; each command is that component's own `just` recipe, so stackctl stays thin
// and the components keep owning how they build/run.
//
// A shared dev secret lets the dispatcher's bots (dev-stub auth) connect to the
// server, which accepts them alongside human DB logins because STARRAID_DEV_SECRET
// is set (composite auth — see server/cmd/server). This is why the NPC count works
// out of the box; unset it and the server reverts to DB-only auth.
const devSecret = "stackctl-dev"

// Optional services are excluded from "start all" and brought up on demand: the
// Godot client is a GUI window, and npc-bot is a single extra bot for ad-hoc use.
func services() []supervisor.Service {
	env := "STARRAID_DEV_SECRET=" + devSecret + " "
	return []supervisor.Service{
		{Name: "postgres", Dir: "server", Kind: supervisor.Oneshot, Run: "just db-up", Stop: "just db-down"},
		{Name: "server", Dir: "server", Kind: supervisor.Daemon, Run: env + "just run"},
		{Name: "admin", Dir: "admin", Kind: supervisor.Daemon, Run: "just build && just run"},
		{Name: "dispatcher", Dir: "npc", Kind: supervisor.Daemon,
			Run: env + "just run-dispatcher -server localhost:60000 -stats :8091 -bots 2"},
		{Name: "client", Dir: "client", Kind: supervisor.Daemon, Run: "just build && just run", Optional: true},
		{Name: "npc-bot", Dir: "npc", Kind: supervisor.Daemon, Run: env + "just run -persist", Optional: true},
	}
}
