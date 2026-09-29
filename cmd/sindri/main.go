// package: main (sindri) / main
// type:    entrypoint (thin)
// job:     wire the container and coding-agent backends this process reads through (the
// hub itself is a separate process — see cmd/sindri-hub), mirror the build version
// into the CLI package, assemble the host CLI command tree (internal/ui/cli) under
// the root, and dispatch.
// limits:  no command logic here — just composition + the version ldflags anchor.
package main

import (
	"fmt"
	"os"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/tools/debug"
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
	// The coding-agent backend too: a front-end renders what a model id MEANS (-> agent.ShortModel)
	// and classifies a captured pane (-> ui/attach.herdrState), both of which are the backend's
	// knowledge. Unwired, those fell to the no-op, which shortens nothing and calls every pane idle.
	agentport.Use(claude.New())
	cli.SetVersion(version) // mirror the ldflags build version into the CLI package
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
	rootCmd.AddCommand(cli.NewMailCmd())
	rootCmd.AddCommand(cli.NewRepoCmd())
	rootCmd.AddCommand(cli.NewTaskCmd())
	rootCmd.AddCommand(cli.NewPrCmd())
	rootCmd.AddCommand(cli.NewRunCmd())
	rootCmd.AddCommand(cli.NewTuiCmd())
	rootCmd.AddCommand(cli.NewUpgradeCmd())
	rootCmd.AddCommand(cli.NewVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
