// Command stackctl is the StarRaid stack control-center. For now it runs headless:
// it brings the stack up, prints a status table, and stops everything cleanly on
// Ctrl-C. The Bubble Tea TUI (service table + logs + CPU/mem + game telemetry)
// replaces this driver in the next slice (see ../../.claude/plans/stackctl-tui.md).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/StarRaidGame/stackctl/internal/supervisor"
)

func main() {
	root := flag.String("root", defaultRoot(), "stack root: the directory holding the component submodules")
	flag.Parse()

	sup := supervisor.New(*root, services())
	fmt.Printf("stackctl — stack root: %s\n", *root)
	fmt.Println("bringing the stack up (Ctrl-C to stop everything)…")
	sup.StartAll()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	printStatus(sup)
	for {
		select {
		case <-sigc:
			fmt.Println("\nstopping the stack…")
			sup.StopAll()
			printStatus(sup)
			return
		case <-tick.C:
			printStatus(sup)
		}
	}
}

// printStatus renders the current service table.
func printStatus(sup *supervisor.Supervisor) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintf(w, "\nSERVICE\tSTATUS\tPID\tUPTIME\n")
	for _, st := range sup.Status() {
		pid, uptime := "—", "—"
		if st.PID > 0 {
			pid = fmt.Sprintf("%d", st.PID)
		}
		if st.Uptime > 0 {
			uptime = st.Uptime.Round(time.Second).String()
		}
		name := st.Name
		if st.Optional {
			name += " *"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, st.State, pid, uptime)
	}
	_ = w.Flush()
}

// defaultRoot finds the stack root: STARRAID_ROOT if set, else the nearest of `..`
// or `.` (or the parent of the cwd when launched from within stackctl/) that holds
// a .gitmodules — the meta repo. Falls back to `..`.
func defaultRoot() string {
	if v := os.Getenv("STARRAID_ROOT"); v != "" {
		return v
	}
	wd, err := os.Getwd()
	if err == nil {
		if filepath.Base(wd) == "stackctl" {
			return filepath.Dir(wd)
		}
		for _, cand := range []string{wd, ".."} {
			if isStackRoot(cand) {
				return cand
			}
		}
	}
	return ".."
}

func isStackRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".gitmodules"))
	return err == nil
}
