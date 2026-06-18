// package: codemap
// type:    dev tool (codebase introspection)
// job:     print a high-signal overview of a Go tree — per file, the structured
//          arch header (the comment block above `package`) plus each type and
//          function with its doc comment and signature, bodies omitted. A map
//          to navigate by without reading whole files.
// limits:  read-only; parses with go/ast; no build/type-checking.
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
	"strings"
)

// skipDirs are trees with no first-party source worth mapping.
var skipDirs = map[string]bool{".git": true, "vendor": true, "node_modules": true, ".worktrees": true}

// Write prints a code map of every .go file under root to w. maxDepth bounds
// how many directory levels below root to descend (0 = root only, 1 = root +
// immediate subdirs, …); a negative maxDepth means unlimited.
//
// fileQ (if non-empty) keeps only files whose path contains it. grepQ (if
// non-empty) keeps only files whose source contains it, and within them only
// the decls that enclose a match (both case-insensitive).
func Write(w io.Writer, root string, maxDepth int, fileQ, grepQ string) error {
	fq, gq := strings.ToLower(fileQ), strings.ToLower(grepQ)
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		rel, e := filepath.Rel(root, path)
		if e != nil {
			rel = path
		}
		if fq != "" && !strings.Contains(strings.ToLower(rel), fq) {
			return nil // filename filter
		}
		writeFile(w, rel, path, gq)
		return nil
	})
}

// dirDepth is how many directory levels path sits below root (root itself = 0).
func dirDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// unit is one mapped declaration: the lines to print plus its source line range
// (doc comment through closing brace), used to test grep hits.
type unit struct {
	lines      []string
	start, end int
}

func writeFile(w io.Writer, rel, path, grepQ string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		if grepQ == "" {
			fmt.Fprintf(w, "\n%s\n  // parse error: %v\n", rel, err)
		}
		return
	}

	var header []string // the arch header (comment block above `package`)
	if f.Doc != nil {
		for _, c := range f.Doc.List {
			header = append(header, c.Text)
		}
	}
	var units []unit
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			units = append(units, unit{append(docLines(d.Doc), "  "+signature(fset, d)),
				startLine(fset, d.Doc, d.Pos()), fset.Position(d.End()).Line})
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				units = append(units, unit{typeUnit(fset, d),
					startLine(fset, d.Doc, d.Pos()), fset.Position(d.End()).Line})
			}
		}
	}

	if grepQ != "" { // keep only decls enclosing a source match; skip the file if none
		hits := matchingLines(path, grepQ)
		if len(hits) == 0 {
			return
		}
		kept := units[:0]
		for _, u := range units {
			if anyInRange(hits, u.start, u.end) {
				kept = append(kept, u)
			}
		}
		units = kept
	}

	fmt.Fprintf(w, "\n%s\n", rel)
	for _, l := range header {
		fmt.Fprintln(w, l)
	}
	for _, u := range units {
		for _, l := range u.lines {
			fmt.Fprintln(w, l)
		}
	}
}

// startLine is a decl's first mapped line — its doc comment if any, else the
// declaration keyword (so a grep hit in the doc counts as enclosed).
func startLine(fset *token.FileSet, doc *ast.CommentGroup, pos token.Pos) int {
	if doc != nil {
		return fset.Position(doc.Pos()).Line
	}
	return fset.Position(pos).Line
}

// matchingLines returns the 1-based line numbers of path whose text contains q
// (q already lowercased).
func matchingLines(path, q string) []int {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var hits []int
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(strings.ToLower(line), q) {
			hits = append(hits, i+1)
		}
	}
	return hits
}

func anyInRange(lines []int, start, end int) bool {
	for _, ln := range lines {
		if ln >= start && ln <= end {
			return true
		}
	}
	return false
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

// typeUnit renders a type declaration as a unit: doc + a one-line
// `type Name kind` per spec.
func typeUnit(fset *token.FileSet, d *ast.GenDecl) []string {
	lines := docLines(d.Doc)
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("  type %s %s", ts.Name.Name, typeKind(fset, ts.Type)))
	}
	return lines
}

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
