// package: main (sindri) / hublist
// type:    command
// job:     wires `sindri hub list` — enumerate every hub running on this machine
//          and the repo each serves, with pid, build version, uptime, and a
//          stale-vs-this-CLI flag. Cross-repo by nature, so it inspects processes
//          (a hub per repo) rather than querying one hub over its socket.
// limits:  discovery/formatting only; pid+socket helpers live in internal/hub.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/spf13/cobra"
)

func newHubListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every hub running on this machine and the repo each serves",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			hubs := runningHubs()
			if len(hubs) == 0 {
				fmt.Println("no hubs running")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "REPO\tPID\tVERSION\tUPTIME\tSTATUS")
			for _, h := range hubs {
				status := "current"
				switch {
				case h.Version == "":
					status = "stale? (predates version stamping)"
				case h.Version != version:
					status = "stale (CLI is " + version + ")"
				}
				fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", h.Root, h.PID, dash(h.Version), dash(h.Uptime), status)
			}
			return tw.Flush()
		},
	}
}

// hubProc is one running hub: the repo it serves, its serving pid, build version
// (from the repo's hub.pid, "" if it predates stamping), and process uptime.
type hubProc struct {
	PID     int
	Root    string
	Version string
	Uptime  string
}

// runningHubs discovers every hub serving a repo on this machine. It takes the
// cwds of `sindri hub` processes as candidate repo roots, keeps those with a live
// control socket, and reports each — deduped by repo, using the socket's actual
// owner as the pid (so a transient `sindri hub list` can't masquerade as a hub).
func runningHubs() []hubProc {
	out, _ := exec.Command("pgrep", "-f", "sindri hub").Output()
	roots := map[string]bool{}
	for _, f := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(f)
		if err != nil {
			continue
		}
		if root := procCwd(pid); root != "" && hub.IsRunning(root) {
			roots[root] = true
		}
	}
	var hubs []hubProc
	for root := range roots {
		pid, _ := hub.HubPID(root) // the real socket owner, not necessarily the pgrep hit
		_, ver, _ := hub.ReadPID(root)
		hubs = append(hubs, hubProc{PID: pid, Root: root, Version: ver, Uptime: procUptime(pid)})
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].Root < hubs[j].Root })
	return hubs
}

// procCwd returns a process's working directory via lsof, or "" if unavailable.
func procCwd(pid int) string {
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
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
