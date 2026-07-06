// Package supervisor runs and monitors a set of local child-process services:
// start/stop/restart, per-service log capture, and lifecycle state tracking. It is
// generic — the StarRaid service list itself lives with the command (see
// cmd/stackctl/manifest.go), so the supervisor knows nothing StarRaid-specific.
//
// Daemon services run as a real child process in their own process group
// (Setpgid), so a stop signals the whole group — cleanly killing `just`→`go
// run`→binary chains that the old `screen`/`pkill -s` orchestration had to fight.
package supervisor

// Kind is how a service is run and stopped.
type Kind int

const (
	// Daemon is a long-running child process: its stdout/stderr are piped into the
	// log buffer, and it is stopped by signalling its process group.
	Daemon Kind = iota
	// Oneshot's Run and Stop are short commands that run to completion (e.g.
	// `docker compose up -d` / `down`); "running" means the up command succeeded.
	Oneshot
)

func (k Kind) String() string {
	if k == Oneshot {
		return "oneshot"
	}
	return "daemon"
}

// State is a service's lifecycle state.
type State int

const (
	Stopped State = iota
	Starting
	Running
	Stopping
	Crashed // exited non-zero (or failed to start) while not being stopped
)

func (s State) String() string {
	switch s {
	case Starting:
		return "starting"
	case Running:
		return "running"
	case Stopping:
		return "stopping"
	case Crashed:
		return "crashed"
	default:
		return "stopped"
	}
}

// Service is one supervised process/command. Run and Stop are shell command
// strings executed via `sh -c` with the working directory set to Dir resolved
// against the stack root.
type Service struct {
	Name     string
	Dir      string // working directory, relative to the stack root
	Run      string // Daemon: the long-running process; Oneshot: the "up" command
	Stop     string // Oneshot only: the "down" command
	Kind     Kind
	Optional bool // excluded from StartAll; started on demand (e.g. the GUI client)
}
