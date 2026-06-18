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
	"path/filepath"
	"strings"
)

// skipDirs are trees with no first-party source worth mapping.
var skipDirs = map[string]bool{".git": true, "vendor": true, "node_modules": true, ".worktrees": true}

// Write prints a code map of every .go file under root to w. maxDepth bounds
// how many directory levels below root to descend (0 = root only, 1 = root +
// immediate subdirs, …); a negative maxDepth means unlimited. When find is
// non-empty, only files whose header or a decl contains it (case-insensitive)
// are printed, and within them only the matching decls.
func Write(w io.Writer, root string, maxDepth int, find string) error {
	q := strings.ToLower(find)
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
		writeFile(w, rel, path, q)
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

func writeFile(w io.Writer, rel, path, q string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		if q == "" {
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
	var units [][]string // one per func/type decl: doc lines + signature/type line(s)
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			units = append(units, append(docLines(d.Doc), "  "+signature(fset, d)))
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				units = append(units, typeUnit(fset, d))
			}
		}
	}

	if q != "" { // filter: keep only matching units; skip the file if nothing hits
		kept := units[:0]
		for _, u := range units {
			if matches(u, q) {
				kept = append(kept, u)
			}
		}
		units = kept
		if len(units) == 0 && !matches(header, q) {
			return
		}
	}

	fmt.Fprintf(w, "\n%s\n", rel)
	for _, l := range header {
		fmt.Fprintln(w, l)
	}
	for _, u := range units {
		for _, l := range u {
			fmt.Fprintln(w, l)
		}
	}
}

// matches reports whether the query (already lowercased) appears in any of the
// lines (case-insensitive).
func matches(lines []string, q string) bool {
	return strings.Contains(strings.ToLower(strings.Join(lines, "\n")), q)
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
