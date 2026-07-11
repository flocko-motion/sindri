// package: main (sindri) / main
// type:    entrypoint (thin)
// job:     wire the container backend, mirror the build version into the CLI
//          package, assemble the host CLI command tree (internal/ui/cli) under the
//          root, and dispatch. The command implementations live in internal/ui/cli;
//          the dev tools (code map, linters) are the separate `brokkr` binary.
// limits:  no command logic here — just composition + the version ldflags anchor.
package main

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/debug"
	"github.com/flo-at/sindri/internal/ui/cli"
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
	cli.SetVersion(version)        // mirror the ldflags build version into the CLI package
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
	rootCmd.AddCommand(cli.NewHubCmd())
	rootCmd.AddCommand(cli.NewCoauthorCmd())
	rootCmd.AddCommand(cli.NewAgentCmd())
	rootCmd.AddCommand(cli.NewChatCmd())
	rootCmd.AddCommand(cli.NewRepoCmd())
	rootCmd.AddCommand(cli.NewTaskCmd())
	rootCmd.AddCommand(cli.NewPrCmd())
	rootCmd.AddCommand(cli.NewTuiCmd())
	rootCmd.AddCommand(cli.NewUpgradeCmd())
	rootCmd.AddCommand(cli.NewVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
