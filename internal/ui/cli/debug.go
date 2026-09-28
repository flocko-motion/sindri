// package: ui/cli / debug
// type:    command (host CLI)
// job:     wires `sindri debug flow`: ask the hub to serve the flow debug view, print its URL, open
// it in the browser in the background, and return to the prompt.
// limits:  the ask and the open. The listener, which lives until the hub stops, is the hub's
// (-> hub/api/debugview).
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/flo-at/sindri/internal/client"
	"github.com/spf13/cobra"
)

// NewDebugCmd builds the `debug` command group.
func NewDebugCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "debug", Short: "Debug views into the running hub"}
	cmd.AddCommand(debugFlowCmd())
	return cmd
}

func debugFlowCmd() *cobra.Command {
	var port int
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Open the flow debug view: every agent's state machine, live, in the browser",
		Long: "Asks the hub to serve its flow debug view on 127.0.0.1 and opens it. The view is " +
			"read-only and stays up until the hub stops; running this again reopens the same URL.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !client.IsRunning() {
				return fmt.Errorf("no hub is running — start one with `sindri hub start --bg`")
			}
			if err := reconcileHubVersion(); err != nil {
				return err
			}
			cl := client.Dial("")
			defer cl.Close()
			url, err := cl.DebugServe(port)
			if err != nil {
				if strings.Contains(err.Error(), "404") {
					return fmt.Errorf("the running hub predates the debug view — restart it: `sindri hub stop` then `sindri hub start --bg`")
				}
				return err
			}
			fmt.Println(url)
			if !noOpen {
				openInBrowser(url)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "port to serve on (default: the hub picks one)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print the URL without opening a browser")
	return cmd
}

// openInBrowser starts the platform's opener detached and does not wait for it. A failure to start
// one is said, since the URL above is then the only way in.
func openInBrowser(url string) {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	c := exec.Command(opener, url)
	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "could not open a browser (%s: %v) — open the URL above yourself\n", opener, err)
		return
	}
	_ = c.Process.Release()
}
