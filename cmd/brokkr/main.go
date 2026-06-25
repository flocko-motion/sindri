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
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

// version is baked in at build time (-X main.version); "dev" for `go run`.
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "brokkr",
		Short:   "brokkr — sindri's toolbelt: code map + linters (no orchestration)",
		Version: fmt.Sprintf("%s (built with %s)", version, runtime.Version()),
		// Bare `brokkr` reports its own version and the Go toolchain it was built
		// with — a linter must run on current Go, so its toolchain is worth seeing
		// — then shows what it can do.
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Printf("brokkr %s (built with %s)\n\n", version, runtime.Version())
			_ = cmd.Help()
		},
	}
	root.AddCommand(newMapCmd(), newLintCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
