// package: main (sindri-worker) / main
// type:    entrypoint (the agent's thin "browser")
// job:     a role-agnostic client with NO built-in subcommands. It dials the hub
//          (a mounted unix socket on Linux, or a token-authed loopback TCP channel
//          on macOS — see dialHub); with no args the hub says what to do next (GET
//          /directive), else it forwards a verb (POST /exec) and streams output.
// limits:  knows no domain logic and no command tree.
package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/flo-at/sindri/internal/hub/client"
)

// version is the build version, baked in via -ldflags (matches the host binary);
// "dev" for a plain build.
var version = "dev"

func main() {
	c := dialHub()
	args := os.Args[1:]

	// `sindri version` is answered locally (not forwarded): it reports THIS binary's
	// build + Go version — the mounted tool the agent actually runs — so a Go-version
	// mismatch with the container (or the host CLI) is diagnosable from inside the pod.
	if len(args) == 1 && args[0] == "version" {
		fmt.Printf("sindri (agent) %s\ngo             %s\n", version, runtime.Version())
		return
	}

	// No args → the hub tells you exactly what to do next (one directive, not a
	// menu). The agent loop is simply: run `sindri`, do what it says. (In the pod
	// this binary is invoked under the name `sindri`.)
	if len(args) == 0 {
		d, err := c.Directive()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
			os.Exit(1)
		}
		fmt.Println(d)
		return
	}

	// `sindri help` lists the verbs available to you RIGHT NOW. The set is
	// computed by the hub from your role and current state, so it changes as you
	// move through the workflow (and may change over time as the system evolves).
	// You normally don't need this — running `sindri` tells you the next step
	// directly.
	if args[0] == "commands" || args[0] == "help" {
		cmds, err := c.Commands()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
			os.Exit(1)
		}
		fmt.Println("Commands available to you right now:")
		for _, cmd := range cmds {
			fmt.Printf("  %-12s %s\n", cmd.Name, cmd.Help)
		}
		fmt.Println("\nThis set is contextual — it depends on your role and current state, so it\nchanges as you work (and may grow over time). Run `sindri help` again whenever\nyou want to see what's available now, and `sindri` (no arguments) for the single\nnext step.")
		return
	}

	exit, err := c.Exec(args, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
		os.Exit(1)
	}
	os.Exit(exit)
}

// dialHub connects to the hub the way the hub told us to: over the loopback TCP
// channel with a token when SINDRI_HUB_ADDR is set (macOS, where the unix socket
// can't cross the podman VM boundary), otherwise over the mounted unix socket.
func dialHub() *client.HTTP {
	if addr := os.Getenv("SINDRI_HUB_ADDR"); addr != "" {
		return client.DialTCP(addr, os.Getenv("SINDRI_TOKEN"))
	}
	sock := os.Getenv("SINDRI_SOCKET")
	if sock == "" {
		sock = "/run/sindri/sock"
	}
	return client.DialSocket(sock)
}
