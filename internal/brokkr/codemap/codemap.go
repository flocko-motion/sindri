// package: codemap
// type:    dev tool (codebase introspection)
// job:     print a high-signal overview of a Go tree — per file, the structured
// arch header (the comment block above `package`) plus each type and
// function with its doc comment and signature, bodies omitted. A map
// to navigate by without reading whole files.
// limits:  read-only; parses with go/ast; no build/type-checking. The two search
// modes (--find context, --grep lines) live in search.go.
package codemap

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

// skipDirs are trees with no first-party source worth mapping.
var skipDirs = map[string]bool{".git": true, "vendor": true, "node_modules": true, ".worktrees": true}

// Query narrows a map. File is a path substring; Find and Grep are smart-case regexps
// (insensitive unless the pattern has an uppercase letter), mutually exclusive: Find keeps
// the decls ENCLOSING a match (structure), Grep emits the matching LINES (locations). Split
// so that wanting the lines doesn't mean piping the map through grep.
//
// Symbol is a third, different kind of search: an exact Go identifier, case-sensitive, never a
// regex or a substring ("Foo" never matches "FooBar") — a lookup, not a text search, and mutually
// exclusive with the other two. Shares that exact-identifier convention with `brokkr refs`, so a
// name means the same thing to both.
type Query struct {
	File   string
	Find   string
	Grep   string
	Symbol string
}

// compiled is a Query with its patterns built once per run rather than per file.
type compiled struct {
	file       string
	find, grep *regexp.Regexp
	symbol     string
}

// compile validates a Query and builds its matchers.
func (q Query) compile() (compiled, error) {
	c := compiled{file: strings.ToLower(q.File), symbol: q.Symbol}
	set := 0
	for _, s := range []string{q.Find, q.Grep, q.Symbol} {
		if s != "" {
			set++
		}
	}
	if set > 1 {
		return c, fmt.Errorf("brokkr map: --find, --grep and --symbol are different searches; pass one")
	}
	var err error
	if q.Find != "" {
		if c.find, err = smartCase(q.Find); err != nil {
			return c, fmt.Errorf("brokkr map --find %q: %w", q.Find, err)
		}
	}
	if q.Grep != "" {
		if c.grep, err = smartCase(q.Grep); err != nil {
			return c, fmt.Errorf("brokkr map --grep %q: %w", q.Grep, err)
		}
	}
	return c, nil
}

// searching reports whether the query is a search rather than a plain map.
func (c compiled) searching() bool { return c.find != nil || c.grep != nil || c.symbol != "" }

// Write maps every .go file under each of roots to w. maxDepth bounds descent below each
// root (0 = root only, negative = unlimited). A root that cannot be walked is a loud error.
func Write(w io.Writer, roots []string, maxDepth int, q Query) error {
	c, err := q.compile()
	if err != nil {
		return err
	}
	return write(w, roots, maxDepth, c, false)
}

// DefaultMaxLines is the line budget above which WriteAdaptive reduces its output.
const DefaultMaxLines = 1000

// WriteAdaptive buffers the map and, past max lines without full, reduces rather than floods
// the terminal: a map drops to per-file headers, grep has no lower resolution so it truncates.
func WriteAdaptive(w io.Writer, roots []string, maxDepth int, q Query, full bool, max int) error {
	c, err := q.compile()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := write(&buf, roots, maxDepth, c, false); err != nil {
		return err
	}
	n := bytes.Count(buf.Bytes(), []byte{'\n'})
	if full || n <= max {
		_, err := w.Write(buf.Bytes())
		return err
	}
	if c.grep != nil {
		return truncate(w, buf.Bytes(), max)
	}
	fmt.Fprintf(w, "The full map is %d lines (over the %d-line budget) — showing per-file headers only.\n", n, max)
	fmt.Fprintf(w, "Narrow the scope (a path, --file, or --find), or pass --full to print everything.\n")
	return write(w, roots, maxDepth, c, true)
}

// truncate prints max lines then names the remainder — an over-broad pattern still answers.
func truncate(w io.Writer, out []byte, max int) error {
	lines := strings.SplitAfter(string(out), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, l := range lines[:max] {
		if _, err := io.WriteString(w, l); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "… %d more matching lines (over the %d-line budget) — narrow the scope (a path, --file, a tighter pattern), or pass --full.\n",
		len(lines)-max, max)
	return err
}

// write walks each root through the shared walk and renders to w; headersOnly keeps only each
// file's arch header. An empty result reports what was scanned instead of printing nothing:
// silence read the same whether the tree held no Go at all or simply no match.
func write(w io.Writer, roots []string, maxDepth int, c compiled, headersOnly bool) error {
	visited, matched, err := walkGoFiles(roots, maxDepth, c.file, func(disp, path string) bool {
		return writeFile(w, disp, path, c, headersOnly)
	})
	if err != nil {
		return fmt.Errorf("brokkr map: %w", err)
	}
	// A plain map answers with every file it read, so only "read nothing" can leave it empty; a
	// search has a pattern to name when the tree WAS read in full and still said nothing.
	if c.searching() {
		reportScan(w, visited, matched, roots, c.what())
	} else if visited == 0 {
		reportScan(w, 0, 0, roots, "")
	}
	return nil
}

// displayPath labels a file: relative to its root, or to cwd for several roots (no collisions).
func displayPath(root, cwd, path string, multi bool) string {
	base := root
	if multi {
		base = cwd
	}
	if rel, err := filepath.Rel(base, path); err == nil {
		return rel
	}
	return path
}

// dirDepth is how many directory levels path sits below root (root itself = 0).
func dirDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// unit is one declaration: the lines to print, its source range (doc through closing brace) for
// match tests, the label a search annotates hits with, whether the map prints it, and the exact
// identifier(s) it declares — names, plural, because a grouped `const ( A; B )` is one unit for
// more than one symbol, and label only ever shows the first (-> selectSymbol).
type unit struct {
	lines      []string
	start, end int
	name       string
	names      []string
	mapped     bool
}

// writeFile renders one file and reports whether it emitted anything — the signal that separates
// "read in full, no match" from "read nothing at all".
func writeFile(w io.Writer, rel, path string, c compiled, headersOnly bool) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		if !c.searching() { // a search reports matches, not the state of files it can't read
			fmt.Fprintf(w, "\n%s\n  // parse error: %v\n", rel, err)
			return true
		}
		return false
	}
	units := collectUnits(fset, f)

	if c.grep != nil { // line search: no map framing, the matches ARE the output
		return writeGrep(w, rel, path, c.grep, units)
	}

	show, loose := mappedOnly(units), []hit(nil)
	switch {
	case c.find != nil:
		var ok bool
		if show, loose, ok = selectFind(path, c.find, units); !ok {
			return false
		}
	case c.symbol != "":
		var ok bool
		if show, ok = selectSymbol(units, c.symbol); !ok {
			return false
		}
	}

	fmt.Fprintf(w, "\n%s\n", rel)
	if f.Doc != nil { // the arch header (comment block above `package`)
		for _, cm := range f.Doc.List {
			fmt.Fprintln(w, cm.Text)
		}
	}
	if headersOnly {
		return true // reduced view: the arch header, none of the declarations
	}
	// Matches no decl covers (arch header, imports); without these --find could render empty.
	for _, h := range loose {
		fmt.Fprintf(w, "  %s  %s\n", lineCol(h.line), strings.TrimSpace(h.text))
	}
	for _, u := range show {
		for _, l := range u.lines {
			fmt.Fprintln(w, l)
		}
	}
	return true
}

// collectUnits turns every top-level decl into a unit. Types and funcs are `mapped` — the map
// proper; vars/consts would bury it, but are collected unmapped so a search can still name the
// line that DECLARES a var rather than only the funcs using it.
func collectUnits(fset *token.FileSet, f *ast.File) []unit {
	var units []unit
	add := func(lines []string, name string, names []string, mapped bool, doc *ast.CommentGroup, d ast.Decl) {
		units = append(units, unit{
			lines: lines, name: name, names: names, mapped: mapped,
			start: startLine(fset, doc, d.Pos()), end: fset.Position(d.End()).Line,
		})
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sig := fmt.Sprintf("  %s  %s", lineCol(fset.Position(d.Pos()).Line), signature(fset, d))
			add(append(docLines(d.Doc), sig), funcLabel(fset, d), []string{d.Name.Name}, true, d.Doc, d)
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE:
				add(typeUnit(fset, d), declLabel(d, "type"), declNames(d), true, d.Doc, d)
			case token.VAR, token.CONST:
				add(valueUnit(fset, d), declLabel(d, d.Tok.String()), declNames(d), false, d.Doc, d)
			}
		}
	}
	return units
}

// mappedOnly keeps the declarations the plain map is made of (types and funcs).
func mappedOnly(units []unit) []unit {
	out := make([]unit, 0, len(units))
	for _, u := range units {
		if u.mapped {
			out = append(out, u)
		}
	}
	return out
}

// startLine is a decl's first line — its doc if any, so a match in the doc counts as enclosed.
func startLine(fset *token.FileSet, doc *ast.CommentGroup, pos token.Pos) int {
	if doc != nil {
		return fset.Position(doc.Pos()).Line
	}
	return fset.Position(pos).Line
}

// docLines returns a doc comment's raw lines, indented (nil if no doc).
func docLines(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	out := make([]string, len(doc.List))
	for i, c := range doc.List {
		out[i] = "  " + c.Text
	}
	return out
}

// signature renders a func declaration without its body.
func signature(fset *token.FileSet, fn *ast.FuncDecl) string {
	stub := &ast.FuncDecl{Recv: fn.Recv, Name: fn.Name, Type: fn.Type} // no Doc, no Body
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, stub); err != nil {
		return "func " + fn.Name.Name + "(…)"
	}
	return b.String()
}

// funcLabel names a func for a search: `func Name`, or `func (T) Name` to keep methods apart.
func funcLabel(fset *token.FileSet, fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return "func " + fn.Name.Name
	}
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, fn.Recv.List[0].Type); err != nil {
		return "func " + fn.Name.Name
	}
	return fmt.Sprintf("func (%s) %s", strings.TrimPrefix(b.String(), "*"), fn.Name.Name)
}

// declLabel names a type/var/const block for a search — its first name, "…" if there are more.
func declLabel(d *ast.GenDecl, kw string) string {
	names := declNames(d)
	switch len(names) {
	case 0:
		return kw
	case 1:
		return kw + " " + names[0]
	}
	return fmt.Sprintf("%s %s…", kw, names[0])
}

// declNames lists every identifier a type/var/const block declares — every name, unlike
// declLabel's display string, which only ever shows the first: a grouped `const ( A; B )` is
// still one unit, but a search for B must still find it (-> selectSymbol).
func declNames(d *ast.GenDecl) []string {
	var names []string
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			names = append(names, s.Name.Name)
		case *ast.ValueSpec:
			for _, n := range s.Names {
				names = append(names, n.Name)
			}
		}
	}
	return names
}

// typeUnit renders a type declaration: doc + one `type Name kind` line per spec.
func typeUnit(fset *token.FileSet, d *ast.GenDecl) []string {
	lines := docLines(d.Doc)
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("  %s  type %s %s", lineCol(fset.Position(ts.Pos()).Line), ts.Name.Name, typeKind(fset, ts.Type)))
	}
	return lines
}

// valueUnit renders a var/const declaration: doc + one line per name, values omitted (a search
// locates the declaration). Only a search prints these — see collectUnits.
func valueUnit(fset *token.FileSet, d *ast.GenDecl) []string {
	lines := docLines(d.Doc)
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, n := range vs.Names {
			lines = append(lines, fmt.Sprintf("  %s  %s %s", lineCol(fset.Position(n.Pos()).Line), d.Tok.String(), n.Name))
		}
	}
	return lines
}

// lineCol renders a line number as a fixed-width gutter, so decl signatures line up.
func lineCol(line int) string { return fmt.Sprintf("%4d", line) }

// typeKind summarizes a type: "struct"/"interface", else the rendered expression.
func typeKind(fset *token.FileSet, expr ast.Expr) string {
	switch expr.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	}
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, expr); err != nil {
		return ""
	}
	return b.String()
}
