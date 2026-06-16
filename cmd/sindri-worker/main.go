// package: main (sindri-worker) / main
// type:    entrypoint (the agent's thin "browser")
// job:     a role-agnostic client with NO built-in subcommands. It dials the
//          hub over its mounted socket (its identity) and either lists what it
//          may currently do (no args) or forwards a verb to the hub for
//          execution, streaming the result. The hub decides everything.
// limits:  knows no domain logic and no command tree — the surface comes from
//          the hub (GET /commands); execution is hub-side (POST /exec).
package main

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/client"
)

func main() {
	sock := os.Getenv("SINDRI_SOCKET")
	if sock == "" {
		sock = "/run/sindri.sock"
	}
	c := client.DialSocket(sock)
	args := os.Args[1:]

	// No args → ask the hub what this agent can do right now (the browser menu).
	if len(args) == 0 {
		cmds, err := c.Commands()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
			os.Exit(1)
		}
		fmt.Println("What you can do right now:")
		for _, cmd := range cmds {
			fmt.Printf("  %-10s %s\n", cmd.Name, cmd.Help)
		}
		fmt.Println("\nRun 'sindri-worker <command>' to act.")
		return
	}

	exit, err := c.Exec(args, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
		os.Exit(1)
	}
	os.Exit(exit)
}
