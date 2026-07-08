// package: main (sindri) / version
// type:    command (host CLI)
// job:     `sindri version` — print the build version AND the Go version the binary
//          was compiled with. The Go version is the diagnostic that matters: the
//          agent's mounted tools (brokkr/worker) run the Go this binary was built
//          with, so a stale toolchain here shows up as stale Go in the agents.
// limits:  read-only; just prints.
package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the sindri build version and the Go version it was compiled with",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "sindri %s\ngo     %s\n", version, runtime.Version())
			return nil
		},
	}
}
