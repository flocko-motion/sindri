// package: lint / ignore
// type:    logic
// job:     compiles the --ignore patterns into a path matcher the linters consult
//          to skip files they shouldn't flag (generated code we can't fix).
// limits:  matches paths only — it doesn't read files or know which linter asks;
//          the pattern syntax is documented on NewIgnore.
package lint

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Ignore matches file paths against the --ignore patterns so a linter can skip
// files it shouldn't report. A nil *Ignore matches nothing.
type Ignore struct {
	pats []ignorePat
}

// ignorePat is one compiled pattern and whether it tests a path's basename
// (a glob with no "/") rather than the whole relative path.
type ignorePat struct {
	re       *regexp.Regexp
	basename bool
}

// NewIgnore compiles the given --ignore patterns. Each is one of:
//   - a glob with no "/" — matched against a file's basename at any depth
//     (e.g. "*.gen.go" ignores every generated file);
//   - a glob containing "/" — matched against the slash-relative path, where
//     "*" spans one path segment and "**" spans several (e.g. "internal/gen/**");
//   - "re:<expr>" — the remainder is a Go regexp searched against the path.
//
// A malformed pattern is a hard error: a bad --ignore must fail loudly, never
// silently match nothing.
func NewIgnore(patterns []string) (*Ignore, error) {
	ig := &Ignore{}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var (
			expr     string
			basename bool
		)
		switch {
		case strings.HasPrefix(p, "re:"):
			expr = strings.TrimPrefix(p, "re:") // used as written — caller anchors
		case strings.Contains(p, "/"):
			expr = globToRegexp(p)
		default:
			expr, basename = globToRegexp(p), true
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("ignore pattern %q: %w", p, err)
		}
		ig.pats = append(ig.pats, ignorePat{re: re, basename: basename})
	}
	return ig, nil
}

// Match reports whether path is excluded by any pattern. The path is normalized
// to a clean, slash-separated form first, so "./a/b.go" and "a/b.go" match alike.
func (ig *Ignore) Match(p string) bool {
	if ig == nil {
		return false
	}
	full := filepath.ToSlash(filepath.Clean(p))
	base := path.Base(full)
	for _, pat := range ig.pats {
		target := full
		if pat.basename {
			target = base
		}
		if pat.re.MatchString(target) {
			return true
		}
	}
	return false
}

// globToRegexp translates a path glob into an anchored regexp: "**/" matches zero
// or more leading directories, a bare "**" any run of characters (including "/"),
// "*" any run except "/", "?" one non-"/" char; every other character is literal.
func globToRegexp(glob string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				if i+2 < len(glob) && glob[i+2] == '/' {
					b.WriteString("(?:.*/)?") // **/ → optional leading dirs
					i += 2
				} else {
					b.WriteString(".*") // ** → any depth
					i++
				}
			} else {
				b.WriteString("[^/]*") // * → within one segment
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return b.String()
}
