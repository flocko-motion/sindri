package lint

import (
	"bytes"
	"strings"
	"testing"
)

// TestGofmtCatchesTheMangledHeader is the case this linter exists for. The repo's four-field header
// wraps its long fields, and gofmt reads a line indented under the one above it as a CODE BLOCK —
// rewriting it with a tab and blank `//` separators, which destroys the header. Two files were
// mangled that way before anything noticed, so the gate has to notice.
func TestGofmtCatchesTheMangledHeader(t *testing.T) {
	aligned := "// package: demo / a\n// type:    logic\n// job:     a wrapped field whose second line is\n" +
		"//          indented under the first, which gofmt turns into a code block.\n" +
		"// limits:  none.\npackage a\n"
	root := writeTree(t, map[string]string{"a.go": aligned})

	var out bytes.Buffer
	found, err := Gofmt([]string{root}, nil, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("an indented continuation must be reported:\n%s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "not gofmt-clean") {
		t.Errorf("say what is wrong:\n%s", got)
	}
	if !strings.Contains(got, "gofmt -w") {
		t.Errorf("say how to fix it:\n%s", got)
	}
	if !strings.Contains(got, "column 3") {
		t.Errorf("name the repo's actual cause, or the fix looks arbitrary:\n%s", got)
	}
}

// TestGofmtAcceptsTheColumn3Header: the style the repo converted to must pass, or the sweep bought
// nothing. Continuations start where any comment line starts, so gofmt has no code block to find.
func TestGofmtAcceptsTheColumn3Header(t *testing.T) {
	clean := "// package: demo / b\n// type:    logic\n// job:     a wrapped field whose second line starts\n" +
		"// at column 3, so gofmt leaves it alone.\n// limits:  none.\npackage b\n"
	root := writeTree(t, map[string]string{"b.go": clean})

	var out bytes.Buffer
	found, err := Gofmt([]string{root}, nil, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("the column-3 style must pass:\n%s", out.String())
	}
}

// TestGofmtNamesTheLine: "not formatted" alone sends you diffing a 700-line file by hand.
func TestGofmtNamesTheLine(t *testing.T) {
	// Bad indentation on the body, well below the header.
	root := writeTree(t, map[string]string{"c.go": "package c\n\nfunc C() {\nreturn\n}\n"})
	var out bytes.Buffer
	if _, err := Gofmt([]string{root}, nil, mustIgnore(t), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "c.go:4") {
		t.Errorf("the first differing line should be named, got:\n%s", out.String())
	}
}

// TestGofmtSkipsUnparseableFiles: a syntax error is the compiler's finding, not a formatting one —
// reporting it here would be a second, confusing voice on the same problem.
func TestGofmtSkipsUnparseableFiles(t *testing.T) {
	root := writeTree(t, map[string]string{"broken.go": "package d\n\nfunc D( {\n"})
	var out bytes.Buffer
	found, err := Gofmt([]string{root}, nil, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("unparseable source is not a formatting finding:\n%s", out.String())
	}
}
