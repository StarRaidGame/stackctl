# stackctl

The StarRaid stack control-center — a Go TUI that runs the whole stack from one place:
start/stop/restart every service, tail each service's logs, show per-process CPU/mem, and (once
the `/stats` endpoints land) live game telemetry (objects, sessions, NPCs vs clients, tick rate).

It replaces the fragile `screen` + `pkill -s` orchestration that used to live in the meta repo's
`justfile`: each service runs as a real child process in its own process group, so stops and
restarts are clean and its stdout/stderr are captured for the log view.

This is the one component that knows about all the others — the service list
(`cmd/stackctl/manifest.go`) points at the sibling submodules (`../server`, `../admin`, `../npc`,
`../client`) and drives each via its own `just` recipes.

## Status

- **Supervisor core + headless runner** — ✅ (this slice). `internal/supervisor` spawns/monitors
  services (daemon + one-shot), captures logs, and kills by process group. `cmd/stackctl` is a
  headless driver: start the stack, print a status table, stop everything on Ctrl-C.
- **Bubble Tea TUI**, per-process **CPU/mem** (gopsutil), and **game telemetry** panes — next.
  See `../.claude/plans/stackctl-tui.md`.

## Usage

```sh
just build        # go build ./...
just run          # headless: bring the stack up, status table, Ctrl-C to stop all
```

`stackctl` resolves the stack root automatically (the dir containing `.gitmodules`); override with
`-root <path>` or `STARRAID_ROOT`. Run `just install`/`just build` in the components first — the
supervisor invokes their `just run` recipes, which compile on demand.
