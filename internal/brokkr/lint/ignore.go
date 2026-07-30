// package: lint / ignore
// type:    logic
// job:     compiles --ignore patterns (and a repo's .brokkrignore file) into a path
// matcher the linters consult to skip files they shouldn't flag (generated
// code we can't fix).
// limits:  matches paths only — it doesn't read files or know which linter asks;
// the pattern syntax is documented on NewIgnore.
package lint

import (
	"errors"
	"fmt"
	"os"
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

// NewIgnore compiles --ignore patterns: a glob without "/" matches a basename at any depth, one
// with "/" matches the relative path ("*" one segment, "**" several), and "re:" prefixes a Go
// regexp. A malformed pattern is a hard error — a bad --ignore must never silently match nothing.
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

// IgnoreFileName is the per-repo ignore file, so an exception lives checked in rather than repeated
// on the command line or embedded in a file that gets regenerated.
const IgnoreFileName = ".brokkrignore"

// LoadIgnoreFile reads NewIgnore-syntax patterns one per line, skipping blanks and '#'. Missing is
// no patterns and no error; unreadable is fatal, since a dropped exception hides violations.
func LoadIgnoreFile(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, IgnoreFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", IgnoreFileName, err)
	}
	var pats []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pats = append(pats, line)
	}
	return pats, nil
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

// globToRegexp anchors a path glob: "**/" is zero or more leading dirs, "**" any run including "/",
// "*" any run except "/", "?" one non-"/" char, everything else literal.
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
