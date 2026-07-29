// package: lint / lang
// type:    logic (source-language recognition)
// job:     decide which files the linters read and how to find their comments — Go plus the
//
//	TypeScript/JavaScript family (.ts .tsx .js .jsx .mjs .cjs, React components
//	included) — and split a file into its header block and its remaining comment
//	blocks, which is what the header and comment-average rules both work from.
//
// limits:  lexical scanning only; Go's semantic checks stay on go/ast (-> comments.go).
package lint

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Lang is a source language brokkr lints.
type Lang int

const (
	LangNone Lang = iota // not a linted source file
	LangGo
	LangTS // the TypeScript/JavaScript family, including .tsx/.jsx React components
)

// LangOf classifies a path by extension. Declaration files (.d.ts) are generated interface
// surfaces rather than hand-authored source, so they are not linted.
func LangOf(path string) Lang {
	if strings.HasSuffix(path, ".d.ts") {
		return LangNone
	}
	switch filepath.Ext(path) {
	case ".go":
		return LangGo
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		return LangTS
	}
	return LangNone
}

// IsTestFile reports whether a path is a test, by each ecosystem's convention. Tests are
// exempt from the header rule: their subject is the file they test.
func IsTestFile(path string) bool {
	base := filepath.Base(path)
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	for _, infix := range []string{".test.", ".spec."} {
		if strings.Contains(base, infix) {
			return true
		}
	}
	// __tests__/ and __mocks__/ directories, the JS convention.
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(path)), "/") {
		if part == "__tests__" || part == "__mocks__" {
			return true
		}
	}
	return false
}

// CommentBlock is one run of comment lines: the line it starts on and how many lines it
// spans.
type CommentBlock struct {
	Line  int
	Lines int
	Text  []string // the comment's content, markers stripped
}

// ScanComments splits a file into its comment blocks, in order. A block runs until CODE ends
// it: comment runs separated only by blank lines are one block, and only the comment lines
// count toward its length.
//
// Blank lines deliberately do not end a block. A long comment broken up by blank lines is
// still one long comment, and if a gap ended a block then any file could halve its measured
// comment length by inserting them — the statistic would measure formatting instead of prose.
//
// The scan is lexical, not a parse: a line whose first non-space characters are a comment
// marker counts. That misreads a marker inside a multi-line string literal, which is rare
// enough in real source to be worth the simplicity — and both rules using this are statistical
// or structural, so a stray block changes no verdict on its own.
func ScanComments(src string) []CommentBlock {
	var out []CommentBlock
	var cur *CommentBlock
	inMulti := false
	add := func(n int, text string) {
		if cur == nil {
			cur = &CommentBlock{Line: n, Lines: 1, Text: []string{text}}
			return
		}
		cur.Lines++
		cur.Text = append(cur.Text, text)
	}
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for i, raw := range strings.Split(src, "\n") {
		line, n := strings.TrimSpace(raw), i+1
		switch {
		case inMulti:
			add(n, strings.TrimSpace(strings.TrimPrefix(line, "*")))
			if strings.Contains(line, "*/") {
				inMulti = false
				flush()
			}
		case strings.HasPrefix(line, "/*"):
			body := strings.TrimPrefix(line, "/*")
			add(n, strings.TrimSpace(body))
			if strings.Contains(body, "*/") {
				flush()
			} else {
				inMulti = true
			}
		case strings.HasPrefix(line, "//"):
			add(n, strings.TrimSpace(strings.TrimPrefix(line, "//")))
		case line == "":
			// A gap holds the block open (see above): the blank line itself is not counted.
		default:
			flush() // code — this is where a comment block genuinely ends
		}
	}
	flush()
	return out
}

// HeaderBlock is the file's header: the first comment block, provided nothing but blank lines
// and imports/directives precede it. Returns false when the file opens with code instead.
func HeaderBlock(blocks []CommentBlock) (CommentBlock, bool) {
	if len(blocks) == 0 {
		return CommentBlock{}, false
	}
	return blocks[0], true
}

// hasGoSources reports whether root contains any Go source, so a Go-specific analysis can skip
// a project written in another language instead of failing it.
func hasGoSources(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable corner — not evidence either way
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if LangOf(path) == LangGo {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// readSource reads a file as text, reporting whether it is readable.
func readSource(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(b), true
}
