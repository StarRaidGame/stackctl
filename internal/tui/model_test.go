package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/StarRaidGame/stackctl/internal/supervisor"
	"github.com/StarRaidGame/stackctl/internal/telemetry"
)

func TestViewRendersSectionsWithoutPanic(t *testing.T) {
	sup := supervisor.New(t.TempDir(), []supervisor.Service{
		{Name: "server", Dir: ".", Kind: supervisor.Daemon, Run: "true"},
		{Name: "admin", Dir: ".", Kind: supervisor.Daemon, Run: "true", Optional: true},
	})
	m := New(sup, ":0")

	if m.View() == "" {
		t.Fatal("view before window size should not be empty")
	}

	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(Model)
	nm, _ = m.Update(refreshMsg{statuses: sup.Status(), stats: telemetry.Stats{OK: false}})
	m = nm.(Model)

	out := m.View()
	for _, want := range []string{"stackctl", "SERVICE", "telemetry", "logs:", "server", "admin *"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view missing %q\n%s", want, out)
		}
	}

	// Move selection and toggle log focus — must not panic or wedge.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = nm.(Model)
	if m.sel != 1 {
		t.Fatalf("down should select index 1, got %d", m.sel)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(Model)
	if !m.focusLogs {
		t.Fatal("tab should focus the log pane")
	}
}
