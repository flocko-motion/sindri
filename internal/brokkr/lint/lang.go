// package: lint / lang
// type:    logic (source-language recognition)
// job:     decide which files the linters read — Go plus the TypeScript/JavaScript family — and
// split each into its header block and remaining comment blocks.
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

// LangOf classifies a path by extension; .d.ts is generated surface, not hand-authored source.
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

// CommentBlock is one run of comment lines: Line/End is where it sits, Lines counts only prose.
type CommentBlock struct {
	Line  int
	End   int
	Lines int
	Text  []string // the comment's content, markers stripped
}

// ScanComments splits a file into comment blocks; only CODE ends one, so gaps can't halve a
// measurement. Delimiters and //go: directives aren't prose; a bare `*` or `//` is.
//
// Lexical, so a comment marker inside a string literal counts as a comment. Tracking raw strings by
// backtick parity was tried and reverted: backtick-heavy code (SQL) flipped the state and left 27
// real comment lines in one file unmeasured. Over-counting an example is the safer error.
func ScanComments(src string) []CommentBlock {
	var out []CommentBlock
	var cur *CommentBlock
	inMulti := false
	// Open on the line the comment OPENS on, so a report points at `/**`, not the prose below.
	open := func(n int) {
		if cur == nil {
			cur = &CommentBlock{Line: n}
		}
		cur.End = n
	}
	count := func(text string) {
		cur.Lines++
		cur.Text = append(cur.Text, text)
	}
	flush := func() {
		if cur != nil {
			if cur.Lines > 0 { // nothing but delimiters is not a comment, and earns no credit
				out = append(out, *cur)
			}
			cur = nil
		}
	}
	for i, raw := range strings.Split(src, "\n") {
		line, n := strings.TrimSpace(raw), i+1
		switch {
		case inMulti:
			open(n)
			body, closing := trimClose(line)
			text := strings.TrimSpace(strings.TrimPrefix(body, "*"))
			// A closing line gives only what precedes `*/`; a bare `*` mid-block is a blank.
			if text != "" || !closing {
				count(text)
			}
			if closing {
				inMulti = false
				flush()
			}
		case strings.HasPrefix(line, "/*"):
			open(n)
			body, closing := trimClose(strings.TrimPrefix(line, "/*"))
			if text := strings.TrimSpace(strings.TrimLeft(body, "*")); text != "" {
				count(text) // `/* text`, or a whole one-line `/** text */`
			}
			if closing {
				flush()
			} else {
				inMulti = true
			}
		case strings.HasPrefix(line, "//"):
			open(n)
			if !isGoDirective(line) {
				count(strings.TrimSpace(strings.TrimPrefix(line, "//")))
			}
		case line == "":
			// A gap holds the block open (see above): the blank line itself is not counted.
		default:
			flush() // code — this is where a comment block genuinely ends
		}
	}
	flush()
	return out
}

// isGoDirective reports whether a line instructs the toolchain (//go:build, //go:embed) rather
// than a reader. No space after the slashes is what separates one from prose mentioning it.
func isGoDirective(line string) bool { return strings.HasPrefix(line, "//go:") }

// trimClose strips a trailing `*/`, reporting whether it was there. Removing it BEFORE the
// leading `*` is what lets a bare `*/` reduce to nothing rather than a stray `/`.
func trimClose(s string) (string, bool) {
	if i := strings.Index(s, "*/"); i >= 0 {
		return strings.TrimSpace(s[:i]), true
	}
	return s, false
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
