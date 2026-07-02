// package: main (sindri) / tui
// type:    command
// job:     wires `sindri tui`, launching the lean hub-client dashboard —
//          auto-starting a background hub first if none is running.
// limits:  the TUI itself lives in internal/tui.
package main

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the Sindri TUI dashboard (a hub client)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !term.IsTerminal(int(os.Stdout.Fd())) {
				return fmt.Errorf("sindri tui requires an interactive terminal")
			}
			root, err := repoRoot()
			if err != nil {
				return err
			}
			if err := ensureHubRunning(root); err != nil { // auto-start a bg hub if none is up
				return err
			}
			if err := reconcileHubVersion(root); err != nil {
				return err
			}
			return tui.Run(root)
		},
	}
}
