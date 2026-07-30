// package: ui/cli / hublogs
// type:    command (host CLI)
// job:     `sindri hub logs` — read the background hub's own log (the state dir's
// hub.log), bounded by default and filterable by agent or substring.
// limits:  reads the file only; the hub writes it (-> startHub). Agent activity lives
// in the store and is served by `sindri agent info`, not here.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/tools/paths"
	"github.com/spf13/cobra"
)

// followPoll is how often --follow checks for appended output.
const followPoll = 300 * time.Millisecond

// newHubLogsCmd builds `hub logs`. It carries its own filtering (--agent, --grep) and bound
// (--lines) on purpose: the log is where a stuck agent is diagnosed, and needing `grep`/`tail`
// to read it means reaching for a pipe the callers here — agents included — cannot use.
func newHubLogsCmd() *cobra.Command {
	var lines int
	var follow, pathOnly bool
	var agent, grep string
	c := &cobra.Command{
		Use:   "logs",
		Short: "Show the hub's log (last 50 lines; --follow to stream, --agent/--grep to filter)",
		Long: "The background hub writes everything it does to the state dir's hub.log:\n" +
			"request lines per agent, and the operator-side errors an agent is never shown.\n\n" +
			"  sindri hub logs                  the last 50 lines\n" +
			"  sindri hub logs -n 200           the last 200\n" +
			"  sindri hub logs --agent dain     only lines mentioning that agent\n" +
			"  sindri hub logs --grep rebase    only lines containing 'rebase'\n" +
			"  sindri hub logs --follow         stream new output as it arrives\n" +
			"  sindri hub logs --path           print the log's path and exit",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			logPath := filepath.Join(paths.StateDir(), "hub.log")
			if pathOnly {
				fmt.Fprintln(cmd.OutOrStdout(), logPath)
				return nil
			}
			f, err := os.Open(logPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("no hub log at %s — the hub has not run in the background yet (a foreground `sindri hub start` logs to its own terminal)", logPath)
				}
				return fmt.Errorf("open hub log: %w", err)
			}
			defer f.Close()
			keep := logFilter(agent, grep)
			data, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("read hub log: %w", err)
			}
			out := cmd.OutOrStdout()
			for _, l := range tailMatching(string(data), keep, lines) {
				fmt.Fprintln(out, l)
			}
			if !follow {
				return nil
			}
			return followLog(f, keep, out)
		},
	}
	c.Flags().IntVarP(&lines, "lines", "n", 50, "show only the last N matching lines (0 for all)")
	c.Flags().BoolVarP(&follow, "follow", "f", false, "keep streaming new output as the hub writes it")
	c.Flags().StringVar(&agent, "agent", "", "show only lines mentioning this agent")
	c.Flags().StringVar(&grep, "grep", "", "show only lines containing this substring (case-insensitive)")
	c.Flags().BoolVar(&pathOnly, "path", false, "print the log file's path and exit")
	return c
}

// logFilter builds the line predicate for --agent/--grep. Both are case-insensitive substring
// tests, and both must hold — narrowing to one agent AND one topic is the common diagnosis.
func logFilter(agent, grep string) func(string) bool {
	agent, grep = strings.ToLower(agent), strings.ToLower(grep)
	return func(line string) bool {
		l := strings.ToLower(line)
		return (agent == "" || strings.Contains(l, agent)) && (grep == "" || strings.Contains(l, grep))
	}
}

// tailMatching returns the last n lines of s that satisfy keep (all of them when n <= 0). Filtering
// happens before the bound, so `-n 50 --agent x` yields 50 of x's lines, not 50 lines that may hold
// none — the bound is on what you asked for.
func tailMatching(s string, keep func(string) bool, n int) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l != "" && keep(l) {
			out = append(out, l)
		}
	}
	if n > 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// followLog streams appended lines from f (already read to EOF) until interrupted. Polling, not
// inotify: the hub appends steadily, so a short poll costs nothing and stays portable.
func followLog(f *os.File, keep func(string) bool, out io.Writer) error {
	var pending string
	for {
		buf := make([]byte, 8192)
		n, err := f.Read(buf)
		if n > 0 {
			pending += string(buf[:n])
			for {
				i := strings.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				if l := pending[:i]; l != "" && keep(l) {
					fmt.Fprintln(out, l)
				}
				pending = pending[i+1:]
			}
			continue // drain what's there before sleeping again
		}
		if err != nil && err != io.EOF {
			return fmt.Errorf("follow hub log: %w", err)
		}
		time.Sleep(followPoll)
	}
}
