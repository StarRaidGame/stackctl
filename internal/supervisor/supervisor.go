package supervisor

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// killGrace is how long a daemon gets to exit after SIGTERM before SIGKILL.
const killGrace = 5 * time.Second

// instance is the live state of one service.
type instance struct {
	svc Service
	log *LogBuffer

	mu        sync.Mutex
	state     State
	pid       int // process-group leader pid while running; 0 otherwise
	startedAt time.Time
	lastErr   error
}

// Status is a point-in-time snapshot of a service for display.
type Status struct {
	Name     string
	Kind     Kind
	Optional bool
	State    State
	PID      int
	Uptime   time.Duration
	Err      error
}

// Supervisor owns a fixed set of services (in declared order) and their live
// state. Methods are safe for concurrent use.
type Supervisor struct {
	root  string
	order []string
	inst  map[string]*instance

	// OnChange, if set, is invoked (asynchronously, without locks held) whenever a
	// service changes state — a hook for a UI to refresh.
	OnChange func()
}

// New builds a supervisor for svcs rooted at root (the directory the services'
// Dir fields resolve against). Services start Stopped.
func New(root string, svcs []Service) *Supervisor {
	s := &Supervisor{root: root, inst: make(map[string]*instance, len(svcs))}
	for _, sv := range svcs {
		s.order = append(s.order, sv.Name)
		s.inst[sv.Name] = &instance{svc: sv, log: NewLogBuffer(1000), state: Stopped}
	}
	return s
}

func (s *Supervisor) notify() {
	if s.OnChange != nil {
		go s.OnChange()
	}
}

func (s *Supervisor) command(sv Service, shell string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", shell)
	cmd.Dir = filepath.Join(s.root, sv.Dir)
	in := s.inst[sv.Name]
	cmd.Stdout, cmd.Stderr = in.log, in.log
	return cmd
}

// Start brings a service up. It is a no-op if the service is already starting,
// running, or stopping. Daemons return once the child is spawned (it runs in the
// background); one-shots return once their up command completes.
func (s *Supervisor) Start(name string) error {
	in := s.inst[name]
	if in == nil {
		return fmt.Errorf("unknown service %q", name)
	}
	if in.svc.Kind == Oneshot {
		return s.startOneshot(in)
	}
	return s.startDaemon(in)
}

func (s *Supervisor) startDaemon(in *instance) error {
	in.mu.Lock()
	if in.state == Starting || in.state == Running || in.state == Stopping {
		in.mu.Unlock()
		return nil
	}
	in.state = Starting
	in.mu.Unlock()

	cmd := s.command(in.svc, in.svc.Run)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // child leads its own group
	if err := cmd.Start(); err != nil {
		in.mu.Lock()
		in.state, in.lastErr = Crashed, err
		in.mu.Unlock()
		s.notify()
		return err
	}

	in.mu.Lock()
	in.pid = cmd.Process.Pid // == the new process-group id (Setpgid)
	in.startedAt = time.Now()
	in.state, in.lastErr = Running, nil
	in.mu.Unlock()
	s.notify()

	go func() {
		err := cmd.Wait()
		in.mu.Lock()
		if in.state == Stopping {
			in.state = Stopped // exit we asked for
		} else {
			in.state, in.lastErr = Crashed, err // exited on its own
		}
		in.pid = 0
		in.mu.Unlock()
		s.notify()
	}()
	return nil
}

func (s *Supervisor) startOneshot(in *instance) error {
	in.mu.Lock()
	if in.state == Running || in.state == Starting {
		in.mu.Unlock()
		return nil
	}
	in.state = Starting
	in.mu.Unlock()
	s.notify()

	err := s.command(in.svc, in.svc.Run).Run()
	in.mu.Lock()
	if err != nil {
		in.state, in.lastErr = Crashed, err
	} else {
		in.state, in.lastErr, in.startedAt = Running, nil, time.Now()
	}
	in.mu.Unlock()
	s.notify()
	return err
}

// Stop takes a service down: signalling a daemon's process group (escalating to
// SIGKILL after a grace period), or running a one-shot's Stop command. A no-op if
// the service is not running.
func (s *Supervisor) Stop(name string) error {
	in := s.inst[name]
	if in == nil {
		return fmt.Errorf("unknown service %q", name)
	}
	if in.svc.Kind == Oneshot {
		return s.stopOneshot(in)
	}
	in.mu.Lock()
	if in.state != Running {
		in.mu.Unlock()
		return nil
	}
	in.state = Stopping
	pgid := in.pid
	in.mu.Unlock()
	s.notify()

	if pgid > 1 {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		go s.escalate(in, pgid)
	}
	return nil
}

func (s *Supervisor) stopOneshot(in *instance) error {
	in.mu.Lock()
	if in.state != Running {
		in.mu.Unlock()
		return nil
	}
	in.state = Stopping
	in.mu.Unlock()
	s.notify()

	err := s.command(in.svc, in.svc.Stop).Run()
	in.mu.Lock()
	in.state, in.lastErr = Stopped, err
	in.mu.Unlock()
	s.notify()
	return err
}

// escalate SIGKILLs the group if the daemon has not exited within killGrace.
func (s *Supervisor) escalate(in *instance, pgid int) {
	time.Sleep(killGrace)
	in.mu.Lock()
	stillStopping := in.state == Stopping
	in.mu.Unlock()
	if stillStopping {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}

// Restart stops the service, waits for it to settle, then starts it again.
func (s *Supervisor) Restart(name string) error {
	if _, ok := s.inst[name]; !ok {
		return fmt.Errorf("unknown service %q", name)
	}
	_ = s.Stop(name)
	s.waitSettled(name, killGrace+3*time.Second)
	return s.Start(name)
}

// StartAll starts every non-optional service in declared order, giving each a beat
// to bind before the next (one-shots already block until done).
func (s *Supervisor) StartAll() {
	for _, name := range s.order {
		in := s.inst[name]
		if in.svc.Optional {
			continue
		}
		_ = s.Start(name)
		if in.svc.Kind == Daemon {
			time.Sleep(300 * time.Millisecond)
		}
	}
}

// StopAll stops every service in reverse declared order and waits for the daemons
// to actually exit (best-effort, bounded).
func (s *Supervisor) StopAll() {
	for i := len(s.order) - 1; i >= 0; i-- {
		_ = s.Stop(s.order[i])
	}
	for i := len(s.order) - 1; i >= 0; i-- {
		s.waitSettled(s.order[i], killGrace+3*time.Second)
	}
}

// waitSettled polls until the service is Stopped/Crashed or the timeout elapses.
func (s *Supervisor) waitSettled(name string, timeout time.Duration) {
	in := s.inst[name]
	if in == nil {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		in.mu.Lock()
		st := in.state
		in.mu.Unlock()
		if st == Stopped || st == Crashed {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// State returns a service's current lifecycle state (Stopped for unknown names).
func (s *Supervisor) State(name string) State {
	in := s.inst[name]
	if in == nil {
		return Stopped
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.state
}

// Status snapshots every service, in declared order.
func (s *Supervisor) Status() []Status {
	out := make([]Status, 0, len(s.order))
	for _, name := range s.order {
		in := s.inst[name]
		in.mu.Lock()
		st := Status{
			Name: in.svc.Name, Kind: in.svc.Kind, Optional: in.svc.Optional,
			State: in.state, PID: in.pid, Err: in.lastErr,
		}
		if in.state == Running && !in.startedAt.IsZero() {
			st.Uptime = time.Since(in.startedAt)
		}
		in.mu.Unlock()
		out = append(out, st)
	}
	return out
}

// Logs returns a snapshot of a service's captured log lines.
func (s *Supervisor) Logs(name string) []string {
	in := s.inst[name]
	if in == nil {
		return nil
	}
	return in.log.Lines()
}

// Names returns the service names in declared order.
func (s *Supervisor) Names() []string {
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}
