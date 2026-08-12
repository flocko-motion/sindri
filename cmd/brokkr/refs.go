// package: main (brokkr) / refs
// type:    command
// job:     wires `brokkr refs <symbol>` — where a symbol is USED, classified and ranked, so
// finding call sites needs no raw grep.
// limits:  logic lives in internal/brokkr/codemap; this only wires flags.
package main

import (
	"github.com/flo-at/sindri/internal/brokkr/codemap"
	"github.com/spf13/cobra"
)

func newRefsCmd() *cobra.Command {
	var depth, limit int
	var file string
	var comments bool
	c := &cobra.Command{
		Use:   "refs <symbol> [path...]",
		Short: "Find where a symbol is used: call sites, classified and ranked, with their enclosing declaration",
		Long: "Find every reference to a symbol across a Go tree — the answer to \"who calls " +
			"this\", which is the most common reason to fall back to `grep -rn`.\n\n" +
			"Each hit is classified by what the symbol is DOING there (definition, call, plain " +
			"reference) and the report is ranked, not file-ordered: the definition first, then " +
			"calls, then other references, with test files one step behind their own kind. So " +
			"the top of the output is the part you asked about, and --limit trims the tail " +
			"rather than the answer.\n\n" +
			"Every hit carries where you landed: the file's arch-header `package:` field and the " +
			"declaration the reference sits in — `path:line: <source>  « pkg · func Caller`.\n\n" +
			"The symbol is an EXACT, case-sensitive identifier: `refs Foo` never answers for " +
			"`FooBar`, and it is not a regexp. For pattern search use `brokkr map --grep` (matching " +
			"lines) or `brokkr map --find` (enclosing declarations); to see the symbol's own " +
			"declaration in context, `brokkr map --symbol <name>`.\n\n" +
			"Comments are prose, not references, so they are excluded unless you pass --comments; " +
			"they then rank last. Matching is syntactic (go/ast, no type checking), so two packages " +
			"declaring the same name both answer — scope it with a path or --file.\n\n" +
			"That syntactic limit is deliberate: refs needs no build, so it answers on code that " +
			"does not compile and on a tree it cannot type-check. When you need the type-aware " +
			"answer instead — which types implement an interface, which of two same-named methods " +
			"a call actually resolves to, a rename that is safe rather than textual — a Go agent " +
			"has gopls' tools (go_symbol_references, go_search, go_diagnostics) served over MCP. " +
			"The two are complementary; neither replaces the other.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roots := args[1:]
			if len(roots) == 0 {
				roots = []string{"."}
			}
			q := codemap.RefQuery{Symbol: args[0], File: file, Comments: comments}
			return codemap.WriteRefs(cmd.OutOrStdout(), roots, depth, q, limit)
		},
	}
	c.Flags().IntVar(&depth, "depth", -1, "max directory levels to descend (0 = given path only; -1 = unlimited)")
	c.Flags().StringVar(&file, "file", "", "only files whose path contains this (case-insensitive)")
	c.Flags().BoolVar(&comments, "comments", false, "also report the symbol named in comments (ranked last)")
	c.Flags().IntVar(&limit, "limit", codemap.DefaultRefLimit, "stop after this many hits, then say how many were withheld (0 = all)")
	return c
}
