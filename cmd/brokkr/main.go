// package: main (brokkr) / main
// type:    entrypoint
// job:     wires brokkr — sindri's toolbelt: the generic, hub-less developer
//          tools (`brokkr map`, `brokkr lint`) that work on any Go repo, with no
//          orchestration power. Named for Sindri's brother, who works the bellows
//          alongside the smith.
// limits:  no hub, no agents, no podman — just the tools; the linters live in
//          internal/lint and the map in internal/codemap.
package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "brokkr",
		Short: "brokkr — sindri's toolbelt: code map + linters (no orchestration)",
	}
	root.AddCommand(newMapCmd(), newLintCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
