// package: main (sindri) / main
// type:    entrypoint
// job:     wires the host CLI command tree — the hub-era verbs (hub, new,
//          launch, tell, attach, agents, merge, prs), the TUI, and lint — and
//          dispatches. Everything is a thin client of the hub.
// limits:  no logic — each command delegates to the hub (in-process or over the
//          socket).
package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var projectDir string
	rootCmd := &cobra.Command{
		Use:   "sindri",
		Short: "Sindri — AI agent orchestrator",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if projectDir != "" {
				return os.Chdir(projectDir)
			}
			return nil
		},
	}
	rootCmd.PersistentFlags().StringVar(&projectDir, "project", "", "Project directory (default: git root from cwd)")

	// Hierarchical command tree: <category> <action>. First-order: hub, tui,
	// lint, code.
	rootCmd.AddCommand(newHubCmd())
	rootCmd.AddCommand(newAgentCmd())
	rootCmd.AddCommand(newTaskCmd())
	rootCmd.AddCommand(newPrCmd())
	rootCmd.AddCommand(newTuiCmd())
	rootCmd.AddCommand(newLintCmd())
	rootCmd.AddCommand(newCodeCmd())

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
