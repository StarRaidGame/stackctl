package supervisor

import (
	"strings"
	"testing"
	"time"
)

// waitState polls until name reaches want or the deadline passes.
func waitState(t *testing.T, s *Supervisor, name string, want State) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.State(name) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("service %q: state %v, want %v", name, s.State(name), want)
}

func TestDaemonLifecycle(t *testing.T) {
	s := New(t.TempDir(), []Service{
		{Name: "echo", Dir: ".", Kind: Daemon, Run: "echo hello; sleep 30"},
	})
	if err := s.Start("echo"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, s, "echo", Running)

	// The captured stdout should carry the line the child printed.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !hasLine(s.Logs("echo"), "hello") {
		time.Sleep(20 * time.Millisecond)
	}
	if !hasLine(s.Logs("echo"), "hello") {
		t.Fatalf("log did not capture child output: %v", s.Logs("echo"))
	}

	if st := s.Status()[0]; st.PID <= 0 {
		t.Fatalf("running daemon should report a PID, got %d", st.PID)
	}

	if err := s.Stop("echo"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitState(t, s, "echo", Stopped) // process group signalled, child reaped
}

func TestDaemonCrashDetected(t *testing.T) {
	s := New(t.TempDir(), []Service{
		{Name: "boom", Dir: ".", Kind: Daemon, Run: "exit 3"},
	})
	if err := s.Start("boom"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Exits non-zero on its own → Crashed, not Stopped.
	waitState(t, s, "boom", Crashed)
}

func TestRestartRunsAgain(t *testing.T) {
	s := New(t.TempDir(), []Service{
		{Name: "echo", Dir: ".", Kind: Daemon, Run: "echo up; sleep 30"},
	})
	if err := s.Start("echo"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, s, "echo", Running)
	first := s.Status()[0].PID

	if err := s.Restart("echo"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	waitState(t, s, "echo", Running)
	if second := s.Status()[0].PID; second == first || second <= 0 {
		t.Fatalf("restart should yield a fresh PID: first=%d second=%d", first, second)
	}
	_ = s.Stop("echo")
	waitState(t, s, "echo", Stopped)
}

func TestOneshotUpDown(t *testing.T) {
	s := New(t.TempDir(), []Service{
		{Name: "one", Dir: ".", Kind: Oneshot, Run: "true", Stop: "true"},
	})
	if err := s.Start("one"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := s.State("one"); got != Running {
		t.Fatalf("after up: state %v, want running", got)
	}
	if err := s.Stop("one"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := s.State("one"); got != Stopped {
		t.Fatalf("after down: state %v, want stopped", got)
	}
}

func TestOneshotUpFailureIsCrashed(t *testing.T) {
	s := New(t.TempDir(), []Service{
		{Name: "bad", Dir: ".", Kind: Oneshot, Run: "exit 1", Stop: "true"},
	})
	if err := s.Start("bad"); err == nil {
		t.Fatalf("Start: want error from failing up command")
	}
	if got := s.State("bad"); got != Crashed {
		t.Fatalf("failed up: state %v, want crashed", got)
	}
}

func TestStartAllSkipsOptional(t *testing.T) {
	s := New(t.TempDir(), []Service{
		{Name: "core", Dir: ".", Kind: Daemon, Run: "sleep 30"},
		{Name: "extra", Dir: ".", Kind: Daemon, Run: "sleep 30", Optional: true},
	})
	s.StartAll()
	waitState(t, s, "core", Running)
	if got := s.State("extra"); got != Stopped {
		t.Fatalf("optional service should not auto-start: state %v", got)
	}
	s.StopAll()
	waitState(t, s, "core", Stopped)
}

func hasLine(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
