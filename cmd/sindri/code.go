// package: main (sindri) / code
// type:    command
// job:     wires `sindri code` — codebase tooling for navigating/understanding
//          the source. Currently `code map`; more subcommands will join it.
// limits:  logic lives in internal/codemap; this only wires flags.
package main

import (
	"github.com/flo-at/sindri/internal/codemap"
	"github.com/spf13/cobra"
)

func newCodeCmd() *cobra.Command {
	c := &cobra.Command{Use: "code", Short: "Codebase tooling (overview, navigation)"}
	c.AddCommand(codeMapCmd())
	return c
}

func codeMapCmd() *cobra.Command {
	var depth int
	c := &cobra.Command{
		Use:   "map [path]",
		Short: "Print a structured overview: per file, the arch header + each type/func with its doc and signature",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return codemap.Write(cmd.OutOrStdout(), root, depth)
		},
	}
	c.Flags().IntVar(&depth, "depth", -1, "max directory levels to descend (0 = given path only; -1 = unlimited)")
	return c
}
