// package: codemap
// type:    dev tool (codebase introspection)
// job:     print a high-signal overview of a Go tree — per file, the structured
//          arch header (the comment block above `package`) plus each type and
//          function with its doc comment and signature, bodies omitted. A map
//          to navigate by without reading whole files.
// limits:  read-only; parses with go/ast; no build/type-checking. The two search
//          modes (--find context, --grep lines) live in search.go.
package codemap

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// skipDirs are trees with no first-party source worth mapping.
var skipDirs = map[string]bool{".git": true, "vendor": true, "node_modules": true, ".worktrees": true}

// Query narrows a map. File is a plain path substring. Find and Grep are regexps
// (smart-case: insensitive unless the pattern carries an uppercase letter) and are
// mutually exclusive — they are two different questions:
//
//	Find — "what code is this part of?" Keeps only the declarations ENCLOSING a
//	       match, rendered as map entries. Answers with structure.
//	Grep — "where exactly does this appear?" Emits the matching LINES, each tagged
//	       with its enclosing declaration. Answers with locations.
//
// Splitting them is deliberate: one flag doing both meant the context view was the
// only view, so anyone who actually wanted the lines had to pipe the map through grep.
type Query struct {
	File string
	Find string
	Grep string
}

// compiled is a Query with its patterns built once per run rather than per file.
type compiled struct {
	file       string
	find, grep *regexp.Regexp
}

// compile validates a Query and builds its matchers.
func (q Query) compile() (compiled, error) {
	c := compiled{file: strings.ToLower(q.File)}
	if q.Find != "" && q.Grep != "" {
		return c, fmt.Errorf("brokkr map: --find and --grep are different searches; pass one")
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
func (c compiled) searching() bool { return c.find != nil || c.grep != nil }

// Write prints a code map of every .go file under each of roots to w. maxDepth
// bounds how many directory levels below each root to descend (0 = root only, 1
// = root + immediate subdirs, …); a negative maxDepth means unlimited.
//
// A single root names files relative to it (concise). With several roots the
// display path is made relative to the working directory instead, so files from
// different roots never collide ambiguously.
//
// A root that does not exist (or cannot be walked) is a loud error, not a silent
// skip — it names the offending path.
func Write(w io.Writer, roots []string, maxDepth int, q Query) error {
	c, err := q.compile()
	if err != nil {
		return err
	}
	return write(w, roots, maxDepth, c, false)
}

// DefaultMaxLines is the line budget above which WriteAdaptive falls back to
// headers-only output unless full is requested.
const DefaultMaxLines = 1000

// WriteAdaptive renders the map, but guards against flooding the terminal: it
// buffers the output and, if that runs past max lines (and full is false), reduces
// it. How it reduces depends on the question — a map falls back to per-file headers,
// which is a real answer at lower resolution; grep output has no such lower
// resolution, so it truncates to the budget and says how many matches it cut. Either
// way it names the way out (narrow the scope, or --full).
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

// truncate prints the first max lines of grep output and reports the remainder, so an
// over-broad pattern still answers with real matches instead of a bare complaint.
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

// write walks each root and renders to w. headersOnly drops each file's
// declarations, leaving just its arch header (the reduced view).
func write(w io.Writer, roots []string, maxDepth int, c compiled, headersOnly bool) error {
	cwd, _ := os.Getwd()
	multi := len(roots) > 1
	// Fail loud, up front: a missing/unreadable root is an error naming the path,
	// not a quiet no-op that looks like "this tree has nothing to map". Validate
	// every root before emitting anything, so a typo in a later arg doesn't print
	// a half-map first.
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			return fmt.Errorf("brokkr map %q: %w", root, err)
		}
	}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return fs.SkipDir
				}
				if maxDepth >= 0 && dirDepth(root, path) > maxDepth {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			disp := displayPath(root, cwd, path, multi)
			if c.file != "" && !strings.Contains(strings.ToLower(disp), c.file) {
				return nil // filename filter
			}
			writeFile(w, disp, path, c, headersOnly)
			return nil
		})
		if err != nil {
			return fmt.Errorf("brokkr map %q: %w", root, err)
		}
	}
	return nil
}

// displayPath is the label shown for a file: relative to its root for a single
// root (concise), else relative to the working directory so files from
// different roots stay unambiguous. Falls back to the raw path if neither
// relativization works.
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

// unit is one mapped declaration: the lines to print, its source line range (doc
// comment through closing brace) used to test matches, and the short name a search
// annotates a hit with. mapped marks the decls the map proper is made of.
type unit struct {
	lines      []string
	start, end int
	name       string
	mapped     bool
}

func writeFile(w io.Writer, rel, path string, c compiled, headersOnly bool) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		if !c.searching() { // a search reports matches, not the state of files it can't read
			fmt.Fprintf(w, "\n%s\n  // parse error: %v\n", rel, err)
		}
		return
	}
	units := collectUnits(fset, f)

	if c.grep != nil { // line search: no map framing, the matches ARE the output
		writeGrep(w, rel, path, c.grep, units)
		return
	}

	show, loose := mappedOnly(units), []hit(nil)
	if c.find != nil {
		var ok bool
		if show, loose, ok = selectFind(path, c.find, units); !ok {
			return
		}
	}

	fmt.Fprintf(w, "\n%s\n", rel)
	if f.Doc != nil { // the arch header (comment block above `package`)
		for _, cm := range f.Doc.List {
			fmt.Fprintln(w, cm.Text)
		}
	}
	if headersOnly {
		return // reduced view: the arch header, none of the declarations
	}
	// Matches that no declaration covers (the arch header, the imports) are printed
	// as bare lines. Without this a file could match --find and then render empty.
	for _, h := range loose {
		fmt.Fprintf(w, "  %s  %s\n", lineCol(h.line), strings.TrimSpace(h.text))
	}
	for _, u := range show {
		for _, l := range u.lines {
			fmt.Fprintln(w, l)
		}
	}
}

// collectUnits turns every top-level declaration into a unit. Types and funcs are
// `mapped` — they are the map proper. Vars and consts are collected but NOT mapped:
// printing them by default would bury the map in package-level plumbing. They still
// have to be here so a search can enclose a hit in one and name it — otherwise a
// query matching a `var` declaration reports the functions that USE it and never the
// line that declares it.
func collectUnits(fset *token.FileSet, f *ast.File) []unit {
	var units []unit
	add := func(lines []string, name string, mapped bool, doc *ast.CommentGroup, d ast.Decl) {
		units = append(units, unit{
			lines: lines, name: name, mapped: mapped,
			start: startLine(fset, doc, d.Pos()), end: fset.Position(d.End()).Line,
		})
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sig := fmt.Sprintf("  %s  %s", lineCol(fset.Position(d.Pos()).Line), signature(fset, d))
			add(append(docLines(d.Doc), sig), funcLabel(fset, d), true, d.Doc, d)
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE:
				add(typeUnit(fset, d), declLabel(d, "type"), true, d.Doc, d)
			case token.VAR, token.CONST:
				add(valueUnit(fset, d), declLabel(d, d.Tok.String()), false, d.Doc, d)
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

// startLine is a decl's first mapped line — its doc comment if any, else the
// declaration keyword (so a match in the doc counts as enclosed).
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

// funcLabel names a func for a search annotation: `func Name`, or `func (T) Name` for
// a method so same-named methods on different types stay distinguishable.
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

// declLabel names a type/var/const block for a search annotation — its first declared
// name, with "…" when the block declares more.
func declLabel(d *ast.GenDecl, kw string) string {
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
	switch len(names) {
	case 0:
		return kw
	case 1:
		return kw + " " + names[0]
	}
	return fmt.Sprintf("%s %s…", kw, names[0])
}

// typeUnit renders a type declaration as a unit: doc + a one-line
// `type Name kind` per spec (each prefixed with its source line).
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

// valueUnit renders a var/const declaration as a unit: doc + a one-line `var Name`
// per declared name. Only a search prints these (see collectUnits), and there the
// point is to locate the declaration, not to restate its value.
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

// lineCol renders a source line number as a fixed-width right-aligned gutter, so
// decl signatures line up (the file is named once in the section header).
func lineCol(line int) string { return fmt.Sprintf("%4d", line) }

// typeKind summarizes a type expression: "struct"/"interface" for composites,
// the rendered expression otherwise (e.g. an alias's target).
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
