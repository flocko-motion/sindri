// package: main (sindri) / main
// type:    entrypoint
// job:     wires the host CLI command tree — the hub-era verbs (hub, agent,
//          task, pr) and the TUI — and dispatches. The generic dev tools (code
//          map, linters) are the separate `brokkr` binary.
// limits:  no logic — each command delegates to the hub (in-process or over the
//          socket).
package main

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/debug"
	"github.com/flo-at/sindri/internal/update"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// version is the build version, baked in via -ldflags "-X main.version=…" (see
// the Makefile). Empty/"dev" for a plain `go build`/`go run` — those skip the
// update check.
var version = "dev"

func main() {
	container.Use(chooseRuntime()) // wire the one container backend for this process
	var projectDir string
	var dbg bool
	rootCmd := &cobra.Command{
		Use:     "sindri",
		Short:   "Sindri — AI agent orchestrator",
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			debug.SetEnabled(dbg) // flip the process-wide verbosity switch first
			// Print our build on every run (to stderr, so it never pollutes piped
			// output) — makes a stale binary obvious at a glance.
			fmt.Fprintf(os.Stderr, "sindri %s\n", version)
			if projectDir != "" {
				return os.Chdir(projectDir)
			}
			return nil
		},
	}
	rootCmd.PersistentFlags().StringVar(&projectDir, "project", "", "Project directory (default: git root from cwd)")
	rootCmd.PersistentFlags().BoolVar(&dbg, "debug", false, "verbose debug logging to stderr (e.g. the exact GitHub calls behind `upgrade`)")

	// Best-effort, once-a-day upgrade check — only when stderr is a terminal, so it
	// never corrupts piped output or nags in CI/scripts.
	if term.IsTerminal(int(os.Stderr.Fd())) {
		update.MaybeNotify(version, os.Stderr)
	}

	// Hierarchical command tree: <category> <action>. The generic dev tools
	// (code map, linters) live in the separate `brokkr` binary, not here.
	rootCmd.AddCommand(newHubCmd())
	rootCmd.AddCommand(newCoauthorCmd())
	rootCmd.AddCommand(newAgentCmd())
	rootCmd.AddCommand(newRepoCmd())
	rootCmd.AddCommand(newTaskCmd())
	rootCmd.AddCommand(newPrCmd())
	rootCmd.AddCommand(newTuiCmd())
	rootCmd.AddCommand(newUpgradeCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// dash renders "-" for an empty string in tabular output.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
