// package: ui/cli / hubstatus
// type:    command
// job:     wire `sindri hub status` (show the one running hub — pid, version,
//          uptime, socket) and `sindri hub stop` (stop it). There is a single
//          global hub per machine, so these operate on it directly.
// limits:  status/stop only; the pid/version/socket helpers live in internal/hub.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/spf13/cobra"
)

func newHubStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the running hub (pid, version, uptime)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hub.IsRunning() {
				fmt.Println("no hub running")
				return nil
			}
			pid, _ := hub.HubPID()
			_, ver, _ := hub.ReadPID()
			status := "current"
			switch {
			case ver == "":
				status = "stale? (predates version stamping)"
			case ver != version:
				status = "stale (CLI is " + version + ")"
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "PID\tVERSION\tUPTIME\tSTATUS\tSOCKET")
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", pid, dash(ver), dash(procUptime(pid)), status, hub.SocketPath())
			return tw.Flush()
		},
	}
}

func newHubStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hub.IsRunning() {
				fmt.Println("no hub running")
				return nil
			}
			pid, ok := hub.HubPID()
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

// procUptime returns a process's elapsed run time via ps, or "" if unavailable.
func procUptime(pid int) string {
	if pid == 0 {
		return ""
	}
	out, err := exec.Command("ps", "-o", "etime=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
