// package: ui/cli / hubstatus
// type:    command
// job:     wire `sindri hub status` (show the one running hub — pid, version,
// uptime, socket) and `sindri hub stop` (stop it). There is a single
// global hub per machine, so these operate on it directly.
// limits:  status/stop only; pid/version discovery lives in internal/client, the
// socket path in internal/tools/paths.
package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/tools/paths"
	"github.com/spf13/cobra"
)

func newHubStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the running hub (pid, version, uptime)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !client.IsRunning() {
				fmt.Println("no hub running")
				return nil
			}
			pid, _ := client.HubPID()
			_, ver, _ := client.ReadPID()
			status := "current"
			switch {
			case ver == "":
				status = "stale? (predates version stamping)"
			case ver != version:
				status = "stale (CLI is " + version + ")"
			}
			// Uptime comes from the board (StartedAt), not the OS: the hub is the process that
			// knows when it came up, not something a client should read off `ps`.
			st, _ := client.Dial("").State()
			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "PID\tVERSION\tUPTIME\tSTATUS\tSOCKET")
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", pid, dash(ver), dash(uptimeSince(st.StartedAt)), status, paths.HubSocket())
			return tw.Flush()
		},
	}
}

// uptimeSince renders how long ago the hub reported starting, in ps's own "etime" shape
// (e.g. "3:07" or "1-02:15:09") so the column reads the same as it always has.
func uptimeSince(startedAt string) string {
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	days := int(d.Hours()) / 24
	h, m, s := int(d.Hours())%24, int(d.Minutes())%60, int(d.Seconds())%60
	if days > 0 {
		return fmt.Sprintf("%d-%02d:%02d:%02d", days, h, m, s)
	}
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func newHubStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !client.IsRunning() {
				fmt.Println("no hub running")
				return nil
			}
			pid, ok := client.HubPID()
			if !ok {
				return fmt.Errorf("couldn't find the running hub's pid to stop it — stop it manually")
			}
			if err := stopHub(pid); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "stopped.")
			return nil
		},
	}
}
