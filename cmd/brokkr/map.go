// package: main (brokkr) / map
// type:    command
// job:     wires `brokkr map` — a structured overview of a Go tree to navigate by
//          (per file: the arch header + each type/func with its doc and signature).
// limits:  logic lives in internal/codemap; this only wires flags.
package main

import (
	"github.com/flo-at/sindri/internal/codemap"
	"github.com/spf13/cobra"
)

func newMapCmd() *cobra.Command {
	var depth int
	var file, grep string
	c := &cobra.Command{
		Use:   "map [path...]",
		Short: "Print a structured overview: per file, the arch header + each type/func with its doc and signature",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			roots := args
			if len(roots) == 0 {
				roots = []string{"."}
			}
			return codemap.Write(cmd.OutOrStdout(), roots, depth, file, grep)
		},
	}
	c.Flags().IntVar(&depth, "depth", -1, "max directory levels to descend (0 = given path only; -1 = unlimited)")
	c.Flags().StringVar(&file, "file", "", "only files whose path contains this (case-insensitive)")
	c.Flags().StringVar(&grep, "grep", "", "only files whose source contains this (case-insensitive); shows just the decls that enclose a match")
	return c
}
