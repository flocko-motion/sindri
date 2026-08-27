// package: ui/cli / agentstates
// type:    command (host CLI)
// job:     `sindri agent states <name>` — the debug state log (sd-a72056): every stored
// write's reason, and every distinct derived-status change.
// limits:  no logic — a thin call into the hub backend, split out of agent.go to keep
// that file under the line cap.
package cli

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/api"
	"github.com/spf13/cobra"
)

// agentStatesCmd surfaces the debug state log: an instrument for diagnosing a puzzling status, not
// a board view (deliberately CLI-only, -> internal/ui/parity_test.go's allowlist).
func agentStatesCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "states <name>",
		Short: "Show an agent's debug state log — writes and derived-status changes, newest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, found *api.AgentView) error {
				evs, err := b.StateLog(found.Name)
				if err != nil {
					return err
				}
				rows, matched := capHead(evs, limit)
				if len(rows) == 0 {
					// state_log is purged on every hub restart, so an empty log is the ORDINARY state
					// of a freshly started hub, not a broken command or a wrong name — a blank screen
					// here would read as either of those.
					fmt.Println("no state changes recorded since the hub started (state_log is purged on every restart)")
					return nil
				}
				fmt.Printf("state log (%d of %d):\n", len(rows), matched)
				for _, e := range rows {
					fmt.Printf("  %s  %-10s %s\n", eventTime(e.TS), e.Reason, oneLine(e.Detail, 100))
				}
				if note := limitNotice("row", len(rows), matched); note != "" {
					fmt.Fprint(os.Stderr, note)
				}
				return nil
			})
		},
	}
	c.Flags().IntVar(&limit, "limit", DefaultListLimit, "show at most this many, newest first (0 = no limit)")
	return c
}
