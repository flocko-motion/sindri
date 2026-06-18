// package: main (sindri-worker) / main
// type:    entrypoint (the agent's thin "browser")
// job:     a role-agnostic client with NO built-in subcommands. It dials the
//          hub over its mounted socket (its identity). With no args the hub
//          tells it exactly what to do next (GET /directive); otherwise it
//          forwards a verb to the hub for execution (POST /exec), streaming the
//          result. The hub decides everything — the agent just obeys.
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
	// menu). The agent loop is simply: run `sindri-worker`, do what it says.
	if len(args) == 0 {
		d, err := c.Directive()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
			os.Exit(1)
		}
		fmt.Println(d)
		return
	}

	// `sindri-worker commands` lists every verb currently available to you.
	if args[0] == "commands" || args[0] == "help" {
		cmds, err := c.Commands()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
			os.Exit(1)
		}
		fmt.Println("Available commands:")
		for _, cmd := range cmds {
			fmt.Printf("  %-10s %s\n", cmd.Name, cmd.Help)
		}
		return
	}

	exit, err := c.Exec(args, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot reach the hub — is it running?", err)
		os.Exit(1)
	}
	os.Exit(exit)
}
