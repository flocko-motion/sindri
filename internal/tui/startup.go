// package: tui / startup
// type:    ui (entrypoint)
// job:     Run — connect to the repo's hub, do the one-shot startup work (status
//          reconcile), open the /events subscription, and hand off to the Bubble
//          Tea program. Split from the model/update loop (tui.go) to keep that file
//          focused.
// limits:  wiring only; the model + update loop live in tui.go.
package tui

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/hub/client"
	"github.com/flo-at/sindri/internal/hub"
)

// Run starts the dashboard against the repo's hub (refuses without one).
func Run(root string) error {
	// Startup breadcrumbs to stderr (before the alt screen takes over) so a hang
	// is attributable to a step rather than silent.
	fmt.Fprintf(os.Stderr, "sindri tui: hub at %s\n", hub.SocketPath())
	if !hub.IsRunning() {
		return fmt.Errorf("no hub running — start one first: 'sindri hub start --bg'")
	}
	fmt.Fprintln(os.Stderr, "sindri tui: connecting to /events…")
	cl := client.Dial(root)
	// One-shot: repair any stale task statuses (in_review with no PR, in_progress
	// with no assignee) at startup, so the board opens honest. Best-effort.
	if err := cl.ReconcileTasks(); err != nil {
		fmt.Fprintf(os.Stderr, "sindri tui: task reconcile skipped: %v\n", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := cl.Watch(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "sindri tui: connected — starting dashboard")
	m := newModel(cl, ch, root)
	m.cancel = cancel
	// A project with an openspec/ folder expects the openspec CLI; warn (once, at
	// startup) if it's absent rather than letting spec features quietly do nothing.
	if spec.Enabled(root) && !spec.CLIInstalled() {
		m.noticeText = openspecMissingNotice
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
