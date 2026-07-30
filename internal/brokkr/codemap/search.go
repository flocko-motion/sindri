// package: codemap / search
// type:    logic (pattern matching)
// job:     the two ways to search a mapped tree — --find keeps the declarations
// ENCLOSING a match (what the hit is part of), --grep emits the matching
// LINES tagged with the decl they sit in (where the hit is), so grep-shaped
// output never needs piping through grep.
// limits:  matching only; the walk and the map rendering are codemap.go's.
package codemap

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// smartCase compiles pat as a regexp, case-insensitively unless it carries an uppercase letter —
// the ripgrep convention, where deliberate capitalisation means you meant it. Being a regexp, a
// literal with metacharacters is the caller's to quote.
func smartCase(pat string) (*regexp.Regexp, error) {
	if !strings.ContainsFunc(pat, unicode.IsUpper) {
		pat = "(?i)" + pat
	}
	return regexp.Compile(pat)
}

// what names the active search, so an empty answer can be acted on rather than just believed.
func (c compiled) what() string {
	switch {
	case c.find != nil:
		return "--find " + c.find.String()
	case c.grep != nil:
		return "--grep " + c.grep.String()
	default:
		return "--symbol " + c.symbol
	}
}

// hit is one matching source line: its 1-based number and its text.
type hit struct {
	line int
	text string
}

// matchingLines returns every line of path that re matches.
func matchingLines(path string, re *regexp.Regexp) []hit {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var hits []hit
	for i, line := range strings.Split(string(src), "\n") {
		if re.MatchString(line) {
			hits = append(hits, hit{i + 1, line})
		}
	}
	return hits
}

// enclosing names the declaration whose source range covers line, "" when the line
// sits outside every declaration (the arch header, the import block, blank stretches).
// Units nest only trivially at top level, so the first cover wins.
func enclosing(units []unit, line int) string {
	for _, u := range units {
		if line >= u.start && line <= u.end {
			return u.name
		}
	}
	return ""
}

// writeGrep renders one file's hits as `path:line: text`, parseable by editor jump tooling, plus the
// enclosing declaration — the point being that grep says a line matched, this says where you landed.
func writeGrep(w io.Writer, rel, path string, re *regexp.Regexp, units []unit) bool {
	lines := matchingLines(path, re)
	for _, h := range lines {
		text := strings.TrimSpace(h.text)
		if name := enclosing(units, h.line); name != "" {
			fmt.Fprintf(w, "%s:%d: %s  « %s\n", rel, h.line, text, name)
			continue
		}
		fmt.Fprintf(w, "%s:%d: %s\n", rel, h.line, text)
	}
	return len(lines) > 0
}

// selectSymbol keeps the units declaring the exact identifier name — case-sensitive, no regex, no
// substring ("Foo" never matches "FooBar"), the one thing --symbol and `brokkr refs` must agree on
// so a name means the same thing to both. Every unit whose names include an exact match is kept,
// since two receivers can share a method name, and a grouped const/var block is one unit for more
// than one symbol (its display label only ever shows the first — that's cosmetic, not identity).
func selectSymbol(units []unit, name string) (kept []unit, ok bool) {
	for _, u := range units {
		for _, n := range u.names {
			if n == name {
				kept = append(kept, u)
				break
			}
		}
	}
	return kept, len(kept) > 0
}

// selectFind keeps the units enclosing a match and returns separately the hits no declaration covers
// (the arch header, the imports) — dropping those silently let a file match and then render empty.
// ok is false when nothing matched, so the caller skips the file.
func selectFind(path string, re *regexp.Regexp, units []unit) (kept []unit, loose []hit, ok bool) {
	hits := matchingLines(path, re)
	if len(hits) == 0 {
		return nil, nil, false
	}
	covered := make([]bool, len(hits))
	for _, u := range units {
		for i, h := range hits {
			if h.line >= u.start && h.line <= u.end {
				covered[i] = true
				if len(kept) == 0 || kept[len(kept)-1].start != u.start {
					kept = append(kept, u)
				}
			}
		}
	}
	for i, h := range hits {
		if !covered[i] {
			loose = append(loose, h)
		}
	}
	return kept, loose, true
}
