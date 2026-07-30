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

// IsTestFile spots a test by each ecosystem's convention; its subject is the file it tests.
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
	At    []int    // source line of each Text entry, so a block can be split and still report honestly
}

// ScanComments splits a file into comment blocks; only CODE ends one, so gaps can't halve a
// measurement. Delimiters and //go: directives aren't prose; a bare `*` or `//` is. Lexical, so a
// marker inside a string counts: tracking raw strings by backtick parity was tried and reverted
// after it left 27 real comment lines unmeasured. Over-counting an example is the safer error.
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
	count := func(n int, text string) {
		cur.Lines++
		cur.Text = append(cur.Text, text)
		cur.At = append(cur.At, n)
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
				count(n, text)
			}
			if closing {
				inMulti = false
				flush()
			}
		case strings.HasPrefix(line, "/*"):
			open(n)
			body, closing := trimClose(strings.TrimPrefix(line, "/*"))
			if text := strings.TrimSpace(strings.TrimLeft(body, "*")); text != "" {
				count(n, text) // `/* text`, or a whole one-line `/** text */`
			}
			if closing {
				flush()
			} else {
				inMulti = true
			}
		case strings.HasPrefix(line, "//"):
			open(n)
			if !isGoDirective(line) {
				count(n, strings.TrimSpace(strings.TrimPrefix(line, "//")))
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

// isGoDirective spots //go:build and friends; no space after the slashes separates one from prose.
func isGoDirective(line string) bool { return strings.HasPrefix(line, "//go:") }

// trimClose strips a trailing `*/`, reporting whether it was there. Removing it BEFORE the
// leading `*` is what lets a bare `*/` reduce to nothing rather than a stray `/`.
func trimClose(s string) (string, bool) {
	if i := strings.Index(s, "*/"); i >= 0 {
		return strings.TrimSpace(s[:i]), true
	}
	return s, false
}

// HeaderBlock is the file's first comment block, or false when the file opens with code.
func HeaderBlock(blocks []CommentBlock) (CommentBlock, bool) {
	if len(blocks) == 0 {
		return CommentBlock{}, false
	}
	return blocks[0], true
}

// SplitHeader divides the first comment block into the four-field header and any free-form prose
// below it. A blank comment line after the fields ends the header — the fields and their wrapped
// continuations are contiguous, so nothing past that gap is header.
//
// The split exists because the header is EXEMPT from the length rules (it must be multi-line), and
// an exemption that covers the whole block pays anyone who parks a paragraph above `package`: the
// prose reads as documentation, escapes the trend, and no rule can see it. Split out, it is
// measured wherever the author puts it (-> CommentAvg), so moving prose in or out of the header
// changes nothing about what it costs.
func SplitHeader(b CommentBlock) (header, prose CommentBlock, hasProse bool) {
	cut := -1
	fields := false
	for i, t := range b.Text {
		if isHeaderFieldLine(t) {
			fields = true
			continue
		}
		// The gap only ends the header once a field has opened it, so a file whose first block is
		// ordinary prose (no header at all) is left whole for the caller to reject as missing.
		if fields && t == "" {
			cut = i
			break
		}
	}
	if cut < 0 {
		return b, CommentBlock{}, false
	}
	rest := sliceBlock(b, cut+1)
	if rest.Lines == 0 {
		return b, CommentBlock{}, false // a trailing blank line is not prose
	}
	return sliceBlock(b, 0, cut), rest, true
}

// sliceBlock rebuilds a block from Text[from:to] (to defaults to the end), carrying the real source
// lines across so a split part still reports where it actually sits.
func sliceBlock(b CommentBlock, from int, to ...int) CommentBlock {
	end := len(b.Text)
	if len(to) > 0 {
		end = to[0]
	}
	out := CommentBlock{Text: b.Text[from:end], At: b.At[from:end]}
	// Trailing blanks belong to the gap, not to either part, so they set neither the count nor End.
	for i, t := range out.Text {
		if t != "" {
			out.Lines++
			out.End = out.At[i]
		}
		if out.Line == 0 && t != "" {
			out.Line = out.At[i]
		}
	}
	return out
}

// isHeaderFieldLine spots a `field:` opener — one of the four, so an invented label is prose (and
// strayFields rejects it) rather than something that silently extends the header.
func isHeaderFieldLine(text string) bool {
	name, _, found := strings.Cut(text, ":")
	return found && isHeaderFieldName(name)
}

// hasGoSources lets a Go-only analysis skip another language's project instead of failing it.
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
