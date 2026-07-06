// Package tui is stackctl's Bubble Tea interface: a service table (status, CPU,
// mem, uptime, pid), a scrollable per-service log pane, and a game-telemetry
// panel, driven by the supervisor + /proc sampler + the server's /stats feed.
package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/StarRaidGame/stackctl/internal/supervisor"
	"github.com/StarRaidGame/stackctl/internal/sysproc"
	"github.com/StarRaidGame/stackctl/internal/telemetry"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	subtleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
	labelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	selectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	focusStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("81")).Padding(0, 1)
)

const telemetryWidth = 22

type tickMsg struct{}

type refreshMsg struct {
	statuses []supervisor.Status
	usage    map[int]sysproc.Usage
	stats    telemetry.Stats
	dstats   telemetry.DispatcherStats
}

type actionDoneMsg struct{}

// Model is the TUI state.
type Model struct {
	sup     *supervisor.Supervisor
	sampler *sysproc.Sampler
	tel     *telemetry.Client
	disp    *telemetry.Dispatcher

	names    []string
	sel      int
	statuses []supervisor.Status
	usage    map[int]sysproc.Usage
	stats    telemetry.Stats
	dstats   telemetry.DispatcherStats

	logView   viewport.Model
	focusLogs bool
	width     int
	height    int
}

// New builds the model. statsAddr is the server's /stats address (e.g. ":8080");
// dispAddr is the npc dispatcher's /stats address (e.g. ":8091").
func New(sup *supervisor.Supervisor, statsAddr, dispAddr string) Model {
	return Model{
		sup:     sup,
		sampler: sysproc.NewSampler(),
		tel:     telemetry.NewClient(statsAddr),
		disp:    telemetry.NewDispatcher(dispAddr),
		names:   sup.Names(),
		logView: viewport.New(60, 10),
	}
}

// Run starts the alt-screen TUI and blocks until the user quits.
func Run(sup *supervisor.Supervisor, statsAddr, dispAddr string) error {
	_, err := tea.NewProgram(New(sup, statsAddr, dispAddr), tea.WithAltScreen()).Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.sup, m.sampler, m.tel, m.disp), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func refreshCmd(sup *supervisor.Supervisor, sm *sysproc.Sampler, tel *telemetry.Client, disp *telemetry.Dispatcher) tea.Cmd {
	return func() tea.Msg {
		st := sup.Status()
		pids := make([]int, 0, len(st))
		for _, s := range st {
			if s.PID > 0 {
				pids = append(pids, s.PID)
			}
		}
		return refreshMsg{
			statuses: st,
			usage:    sm.Sample(pids),
			stats:    tel.Fetch(context.Background()),
			dstats:   disp.Fetch(context.Background()),
		}
	}
}

// action runs a supervisor operation off the UI goroutine, then asks for a refresh.
func action(f func()) tea.Cmd {
	return func() tea.Msg { f(); return actionDoneMsg{} }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tickMsg:
		return m, tea.Batch(refreshCmd(m.sup, m.sampler, m.tel, m.disp), tickCmd())

	case refreshMsg:
		m.statuses, m.usage, m.stats, m.dstats = msg.statuses, msg.usage, msg.stats, msg.dstats
		m.syncLogs()
		return m, nil

	case actionDoneMsg:
		return m, refreshCmd(m.sup, m.sampler, m.tel, m.disp)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.focusLogs = !m.focusLogs
		return m, nil
	}

	if m.focusLogs {
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "up", "k":
		if m.sel > 0 {
			m.sel--
			m.syncLogs()
		}
	case "down", "j":
		if m.sel < len(m.names)-1 {
			m.sel++
			m.syncLogs()
		}
	case "enter":
		m.focusLogs = true
	case "s":
		return m, action(func() { _ = m.sup.Start(m.cur()) })
	case "x":
		return m, action(func() { _ = m.sup.Stop(m.cur()) })
	case "r":
		return m, action(func() { _ = m.sup.Restart(m.cur()) })
	case "S":
		return m, action(m.sup.StartAll)
	case "X":
		return m, action(m.sup.StopAll)
	}
	return m, nil
}

func (m Model) cur() string { return m.names[m.sel] }

func (m *Model) resize() {
	logW := m.width - telemetryWidth - 8
	if logW < 24 {
		logW = 24
	}
	logH := m.height - len(m.names) - 10
	if logH < 3 {
		logH = 3
	}
	m.logView.Width = logW
	m.logView.Height = logH
	m.syncLogs()
}

func (m *Model) syncLogs() {
	if len(m.names) == 0 {
		return
	}
	m.logView.SetContent(strings.Join(m.sup.Logs(m.cur()), "\n"))
	if !m.focusLogs {
		m.logView.GotoBottom()
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "starting…"
	}
	title := titleStyle.Render("stackctl") + "  " + subtleStyle.Render("StarRaid stack control")
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, m.renderTelemetry(), "  ", m.renderLogs())
	help := helpStyle.Render("↑↓ select · s/x/r start·stop·restart · S/X all · tab logs · q quit")
	return lipgloss.JoinVertical(lipgloss.Left, title, "", m.renderTable(), "", bottom, "", help)
}

func (m Model) renderTable() string {
	cols := []int{2, 12, 9, 7, 8, 8, 7}
	head := row(cols, headerStyle, "", "SERVICE", "STATUS", "CPU", "MEM", "UPTIME", "PID")
	lines := []string{head}
	for i, s := range m.statuses {
		cursor := ""
		if i == m.sel {
			cursor = selectStyle.Render("▸")
		}
		name := s.Name
		if s.Optional {
			name += " *"
		}
		cpu, mem, pid := "—", "—", "—"
		if s.PID > 0 {
			pid = strconv.Itoa(s.PID)
			if u, ok := m.usage[s.PID]; ok {
				cpu = fmt.Sprintf("%.1f%%", u.CPUPercent)
				mem = formatBytes(u.RSSBytes)
			}
		}
		up := "—"
		if s.Uptime > 0 {
			up = formatDur(s.Uptime)
		}
		st := lipgloss.NewStyle().Foreground(statusColor(s.State))
		lines = append(lines, rowMixed(cols, cursor, name, st.Render(s.State.String()), cpu, mem, up, pid))
	}
	return boxStyle.Render(strings.Join(lines, "\n"))
}

func (m Model) renderTelemetry() string {
	st := m.stats
	server := errStyle.Render("down")
	if st.OK {
		server = okStyle.Render("up")
	}
	// sessions are all connections; npcs come from the dispatcher; clients are the
	// remainder (humans). Each is "—" until its source answers.
	npcs := "—"
	if m.dstats.OK {
		npcs = strconv.Itoa(m.dstats.NpcsActive)
	}
	clients := "—"
	if st.OK {
		c := int(st.Sessions)
		if m.dstats.OK {
			if c -= m.dstats.NpcsActive; c < 0 {
				c = 0
			}
		}
		clients = strconv.Itoa(c)
	}
	lines := []string{
		labelStyle.Render("telemetry"),
		"",
		kv("server", server),
		kv("objects", numOrDash(st.OK, st.Objects)),
		kv("sessions", numOrDash(st.OK, int(st.Sessions))),
		kv(" clients", clients),
		kv(" npcs", npcs),
		kv("tick", tickStr(st)),
	}
	return boxStyle.Width(telemetryWidth).Render(strings.Join(lines, "\n"))
}

func (m Model) renderLogs() string {
	style := boxStyle
	if m.focusLogs {
		style = focusStyle
	}
	head := labelStyle.Render("logs: " + m.cur())
	return style.Width(m.logView.Width).Render(head + "\n" + m.logView.View())
}

// row renders header/label cells of fixed visible widths with a single style.
func row(widths []int, style lipgloss.Style, cells ...string) string {
	rendered := make([]string, len(cells))
	for i, c := range cells {
		rendered[i] = style.Width(widths[i]).Render(c)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

// rowMixed renders a data row; the status cell arrives pre-styled (coloured) so it
// is width-boxed without re-styling.
func rowMixed(widths []int, cursor, name, status, cpu, mem, up, pid string) string {
	plain := lipgloss.NewStyle()
	return lipgloss.JoinHorizontal(lipgloss.Top,
		plain.Width(widths[0]).Render(cursor),
		plain.Width(widths[1]).Render(name),
		lipgloss.NewStyle().Width(widths[2]).Render(status),
		plain.Width(widths[3]).Render(cpu),
		plain.Width(widths[4]).Render(mem),
		plain.Width(widths[5]).Render(up),
		plain.Width(widths[6]).Render(pid),
	)
}

func kv(k, v string) string {
	return subtleStyle.Render(fmt.Sprintf("%-8s", k)) + v
}

func numOrDash(ok bool, n int) string {
	if !ok {
		return "—"
	}
	return strconv.Itoa(n)
}

func tickStr(s telemetry.Stats) string {
	if !s.OK {
		return "—"
	}
	return fmt.Sprintf("%.0fHz", s.TickHz)
}

func statusColor(st supervisor.State) lipgloss.Color {
	switch st {
	case supervisor.Running:
		return lipgloss.Color("42")
	case supervisor.Crashed:
		return lipgloss.Color("196")
	case supervisor.Starting, supervisor.Stopping:
		return lipgloss.Color("214")
	default:
		return lipgloss.Color("244")
	}
}

func formatBytes(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0fM", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0fK", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func formatDur(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	mnt := d / time.Minute
	sec := (d - mnt*time.Minute) / time.Second
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, mnt, sec)
	}
	return fmt.Sprintf("%02d:%02d", mnt, sec)
}
