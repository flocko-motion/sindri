// package: main (sindri-worker) / main
// type:    entrypoint (the agent's thin "browser")
// job:     a role-agnostic client with NO built-in subcommands. It dials the
//          hub over its mounted socket (its identity); with no args the hub says
//          what to do next (GET /directive), else it forwards a verb to the hub
//          (POST /exec) and streams the result. The hub decides; the agent obeys.
// limits:  knows no domain logic and no command tree.
package main

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/client"
)

func main() {
	sock := os.Getenv("SINDRI_SOCKET")
	if sock == "" {
		sock = "/run/sindri/sock"
	}
	c := client.DialSocket(sock)
	args := os.Args[1:]

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
		fmt.Println("\nThis set is contextual — it depends on your role and current state and can\nchange over time. Run `sindri` (no arguments) for the single next step.")
		return
	}

	exit, err := c.Exec(args, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
		os.Exit(1)
	}
	os.Exit(exit)
}
