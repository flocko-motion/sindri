// package: main (sindri) / tui
// type:    command
// job:     wires `sindri tui`, launching the Bubble Tea program.
// limits:  the TUI itself lives in internal/tui; nothing here but wiring.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/tui"
	"github.com/flo-at/sindri/internal/worker"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the Sindri TUI dashboard",
		RunE:  runTui,
	}
}

func runTui(cmd *cobra.Command, args []string) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("sindri tui requires an interactive terminal")
	}

	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	model := tui.New(projectRoot)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
