// Command stackctl is the StarRaid stack control-center. By default it opens the
// Bubble Tea TUI (service table + logs + CPU/mem + game telemetry); with -headless
// (or when stdout is not a terminal) it brings the stack up, prints a status
// table, and stops everything cleanly on Ctrl-C. See ../../.claude/plans/stackctl-tui.md.
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
	"github.com/StarRaidGame/stackctl/internal/tui"
)

func main() {
	root := flag.String("root", defaultRoot(), "stack root: the directory holding the component submodules")
	headless := flag.Bool("headless", false, "no TUI: bring the stack up, print status, Ctrl-C to stop")
	statsAddr := flag.String("stats", statsAddr(), "server /stats address for the telemetry pane")
	flag.Parse()

	sup := supervisor.New(*root, services())

	if *headless || !isTerminal() {
		runHeadless(sup, *root)
		return
	}
	if err := tui.Run(sup, *statsAddr); err != nil {
		fmt.Fprintln(os.Stderr, "tui error:", err)
		os.Exit(1)
	}
	// The TUI has exited — stop the stack so nothing is left orphaned.
	fmt.Println("stopping the stack…")
	sup.StopAll()
}

// runHeadless brings the non-optional services up, prints a status table on a
// ticker, and stops everything on SIGINT/SIGTERM. This is the CI / no-TTY path.
func runHeadless(sup *supervisor.Supervisor, root string) {
	fmt.Printf("stackctl (headless) — stack root: %s\n", root)
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

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// statsAddr is the server's /stats address, matching the server's STARRAID_ADMIN.
func statsAddr() string {
	if v := os.Getenv("STARRAID_ADMIN"); v != "" {
		return v
	}
	return ":8080"
}

// defaultRoot finds the stack root: STARRAID_ROOT if set, else the parent of the
// cwd when launched from within stackctl/, else the nearest of `.`/`..` holding a
// .gitmodules (the meta repo). Falls back to `..`.
func defaultRoot() string {
	if v := os.Getenv("STARRAID_ROOT"); v != "" {
		return v
	}
	if wd, err := os.Getwd(); err == nil {
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
