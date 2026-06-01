// package: main (sindri) / tui
// type:    command
// job:     wires `sindri tui`, launching the Bubble Tea program. With
//          --script, runs the TUI replay engine instead (headless) and writes
//          captured frames into --capture-dir.
// limits:  the TUI itself lives in internal/tui; the replay engine in
//          internal/tui/replay.go.
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
	var scriptPath, captureDir string
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the Sindri TUI dashboard (or replay a script with --script)",
		Long: `Launch the Sindri TUI dashboard.

Use --script <file> --capture-dir <dir> to drive the TUI headlessly against a
deterministic fixture. The script is a small DSL:

  letters/digits     literal keypresses (e.g. "hello" types 5 keys)
  down up left right special movement keys
  enter esc tab space backspace
  ctrl+x             control combos (a–z)
  (resize W H)       inject a window-resize message
  (drain)            run pending tea.Cmd to quiescence
  (sleep N)          alias for (drain) — no real time is consumed
  (capture <name>)   write <dir>/<name>.ansi and <dir>/<name>.txt

Captures use the SimpleFixture and the engine forces truecolor, so the
.ansi files look like a real terminal session and the .txt files are
diff-friendly. No TTY is required in --script mode.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if scriptPath != "" {
				return runTuiScript(scriptPath, captureDir)
			}
			return runTui(cmd, args)
		},
	}
	cmd.Flags().StringVar(&scriptPath, "script", "", "Replay the TUI from a script file instead of launching interactively")
	cmd.Flags().StringVar(&captureDir, "capture-dir", "", "Where (capture <name>) writes frames (required with --script if you want captures)")
	return cmd
}

func runTui(cmd *cobra.Command, args []string) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("sindri tui requires an interactive terminal (or use --script <file>)")
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

func runTuiScript(scriptPath, captureDir string) error {
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("read script: %w", err)
	}
	return tui.Replay(string(script), tui.SimpleFixture(), captureDir)
}
