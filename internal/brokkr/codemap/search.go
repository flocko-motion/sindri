// package: codemap / search
// type:    logic (pattern matching)
// job:     the two ways to search a mapped tree — --find keeps the declarations
//          ENCLOSING a match (what the hit is part of), --grep emits the matching
//          LINES tagged with the decl they sit in (where the hit is), so grep-shaped
//          output never needs piping through grep.
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

// smartCase compiles pat as a regexp, case-insensitively unless it carries an
// uppercase letter — the ripgrep convention. A lowercase query stays the loose search
// the old substring --grep was, while deliberate capitalisation means you meant it.
// Patterns are regexps now, so a literal with metacharacters needs regexp.QuoteMeta
// treatment by the caller (or backslashes on the command line).
func smartCase(pat string) (*regexp.Regexp, error) {
	if !strings.ContainsFunc(pat, unicode.IsUpper) {
		pat = "(?i)" + pat
	}
	return regexp.Compile(pat)
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

// writeGrep renders the line search for one file: `path:line: text` — a prefix that
// stays parseable by the usual editor/jump tooling — with the enclosing declaration
// appended. That suffix is the whole point: plain grep tells you a line matched,
// this tells you which function or type you just landed in.
func writeGrep(w io.Writer, rel, path string, re *regexp.Regexp, units []unit) {
	for _, h := range matchingLines(path, re) {
		text := strings.TrimSpace(h.text)
		if name := enclosing(units, h.line); name != "" {
			fmt.Fprintf(w, "%s:%d: %s  « %s\n", rel, h.line, text, name)
			continue
		}
		fmt.Fprintf(w, "%s:%d: %s\n", rel, h.line, text)
	}
}

// selectFind narrows units to those enclosing a match and reports the matches that no
// declaration covers (a hit in the arch header or the imports). Returning those
// separately is what keeps the context search honest: the old behaviour silently
// dropped them, so a file could match the query and then render with nothing in it —
// you saw a hit existed but never where. ok is false when the file has no match at
// all, so the caller skips it entirely.
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
