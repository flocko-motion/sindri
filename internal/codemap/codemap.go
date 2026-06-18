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
// immediate subdirs, …); a negative maxDepth means unlimited.
func Write(w io.Writer, root string, maxDepth int) error {
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
		writeFile(w, rel, path)
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

func writeFile(w io.Writer, rel, path string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		fmt.Fprintf(w, "\n%s\n  // parse error: %v\n", rel, err)
		return
	}
	fmt.Fprintf(w, "\n%s\n", rel)
	if f.Doc != nil { // the arch header (comment block above `package`)
		for _, c := range f.Doc.List {
			fmt.Fprintf(w, "%s\n", c.Text)
		}
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			writeDoc(w, d.Doc)
			fmt.Fprintf(w, "  %s\n", signature(fset, d))
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				writeType(w, fset, d)
			}
		}
	}
}

// writeDoc prints a declaration's doc comment, indented.
func writeDoc(w io.Writer, doc *ast.CommentGroup) {
	if doc == nil {
		return
	}
	for _, c := range doc.List {
		fmt.Fprintf(w, "  %s\n", c.Text)
	}
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

// writeType prints type declarations (doc + a one-line `type Name kind`).
func writeType(w io.Writer, fset *token.FileSet, d *ast.GenDecl) {
	writeDoc(w, d.Doc)
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		fmt.Fprintf(w, "  type %s %s\n", ts.Name.Name, typeKind(fset, ts.Type))
	}
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
