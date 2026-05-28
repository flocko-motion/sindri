// package: main (gh) / main
// type:    entrypoint
// job:     sindri-local, the agent-facing workflow CLI used in containers (NOT
//          GitHub); dispatches the gh command tree.
// limits:  all commands live in internal/ghlocal/cmd; nothing here but dispatch.
package main

import "github.com/flo-at/sindri/internal/ghlocal/cmd"

func main() {
	cmd.Execute()
}
