// package: main (brokkr) / map
// type:    command
// job:     wires `brokkr map` — a structured overview of a Go tree to navigate by
//          (per file: the arch header + each type/func with its doc and signature).
// limits:  logic lives in internal/codemap; this only wires flags.
package main

import (
	"github.com/flo-at/sindri/internal/brokkr/codemap"
	"github.com/spf13/cobra"
)

func newMapCmd() *cobra.Command {
	var depth, max int
	var file, grep string
	var full bool
	c := &cobra.Command{
		Use:   "map [path...]",
		Short: "Print a structured overview: per file, the arch header + each type/func with its doc and signature",
		Long: "Print a structured overview of a Go tree — per file, the arch header plus " +
			"each type/func with its doc and signature. If the full output runs past " +
			"--max lines it reduces to per-file headers only (with a note); --full prints " +
			"everything regardless.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			roots := args
			if len(roots) == 0 {
				roots = []string{"."}
			}
			return codemap.WriteAdaptive(cmd.OutOrStdout(), roots, depth, file, grep, full, max)
		},
	}
	c.Flags().IntVar(&depth, "depth", -1, "max directory levels to descend (0 = given path only; -1 = unlimited)")
	c.Flags().StringVar(&file, "file", "", "only files whose path contains this (case-insensitive)")
	c.Flags().StringVar(&grep, "grep", "", "only files whose source contains this (case-insensitive); shows just the decls that enclose a match")
	c.Flags().BoolVar(&full, "full", false, "print everything, however long (no reduce-to-headers)")
	c.Flags().IntVar(&max, "max", codemap.DefaultMaxLines, "line budget before reducing to per-file headers")
	return c
}
