package codemap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// refTree writes a small module whose every interesting reference shape appears exactly once, so
// a classification test can name the line it expects.
func refTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"lib/lib.go": `// package: lib / lib
// type:    logic
// job:     the thing under search.
// limits:  none.
package lib

// Target is the symbol every other file refers to.
func Target(n int) int { return n }

// Holder has a field named Target, which is its own declaration of the name.
type Holder struct{ Target int }

// Caller calls it.
func Caller() int { return Target(1) }

// Aliased is a package-level var: a definition AND a reference on one line.
var Aliased = Target

// Local shadows nothing but declares a local of the same spelling — a use, not a definition.
func Local() int {
	Target := 2
	return Target
}
`,
		"app/app.go": `// package: app / app
// type:    logic
// job:     a cross-package caller.
// limits:  none.
package app

import "m/lib"

// Use calls it through a selector, and passes it as a value.
func Use() int {
	f := lib.Target
	return lib.Target(3) + f(4)
}
`,
		"app/other.go": `// package: app / other
// type:    logic
// job:     a production caller of Use, so ranking has something to order.
// limits:  none.
package app

// Again calls Use from production code.
func Again() int { return Use() }
`,
		"app/app_test.go": `package app

import "testing"

// TestUse calls it from a test — live code, ranked behind production.
func TestUse(t *testing.T) {
	if Use() == 0 {
		t.Fatal("no")
	}
}
`,
		"noise/noise.go": `// package: noise / noise
// type:    logic
// job:     names that must NOT match.
// limits:  none.
package noise

// TargetSuffix and prefixTarget are different identifiers; Target in prose is not a reference.
func TargetSuffix() {}
func prefixTarget() {}
`,
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// find returns the refs at a path suffix and line, so a case can assert on one exact occurrence.
func find(refs []Ref, pathSuffix string, line int) []Ref {
	var out []Ref
	for _, r := range refs {
		if strings.HasSuffix(filepath.ToSlash(r.Path), pathSuffix) && r.Line == line {
			out = append(out, r)
		}
	}
	return out
}

// TestRefsClassifies is the heart of it: the same identifier means different things in different
// places, and telling them apart is what a regex line search cannot do.
func TestRefsClassifies(t *testing.T) {
	scan, err := Refs([]string{refTree(t)}, -1, RefQuery{Symbol: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	refs := scan.Refs
	cases := []struct {
		what string
		path string
		line int
		kind RefKind
	}{
		{"the func declaration", "lib/lib.go", 8, RefDef},
		{"a struct field declaring the name", "lib/lib.go", 11, RefDef},
		{"a bare call", "lib/lib.go", 14, RefCall},
		{"a selector call", "app/app.go", 12, RefCall},
		{"passed as a value", "app/app.go", 11, RefOther},
	}
	for _, c := range cases {
		got := find(refs, c.path, c.line)
		if len(got) != 1 {
			t.Errorf("%s (%s:%d): want 1 hit, got %d", c.what, c.path, c.line, len(got))
			continue
		}
		if got[0].Kind != c.kind {
			t.Errorf("%s (%s:%d) = %v, want %v", c.what, c.path, c.line, got[0].Kind, c.kind)
		}
	}
}

// TestRefsSeparatesTwoHitsOnOneLine: `var Aliased = Target` declares nothing called Target, but
// `var Target = Target` would — the point is that one line can hold two different kinds, which is
// why the report carries columns.
func TestRefsSeparatesTwoHitsOnOneLine(t *testing.T) {
	scan, err := Refs([]string{refTree(t)}, -1, RefQuery{Symbol: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	refs := scan.Refs
	got := find(refs, "lib/lib.go", 17) // var Aliased = Target
	if len(got) != 1 || got[0].Kind != RefOther {
		t.Fatalf("the RHS of an alias var is a plain reference, got %+v", got)
	}
	if got[0].Col == 0 {
		t.Error("a hit must carry its column, or two hits on one line are indistinguishable")
	}
}

// TestRefsIsExactAndCaseSensitive pins the semantics agreed with `map --symbol`: an identifier,
// not a pattern. TargetSuffix/prefixTarget share the substring and must never answer.
func TestRefsIsExactAndCaseSensitive(t *testing.T) {
	dir := refTree(t)
	scan, err := Refs([]string{dir}, -1, RefQuery{Symbol: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	refs := scan.Refs
	for _, r := range refs {
		if strings.Contains(filepath.ToSlash(r.Path), "noise/") {
			t.Errorf("substring match leaked in: %s:%d %s", r.Path, r.Line, r.Text)
		}
	}
	// Case matters, so the lowercase spelling finds nothing here.
	lower, err := Refs([]string{dir}, -1, RefQuery{Symbol: "target"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lower.Refs) != 0 {
		t.Errorf("case-insensitive match leaked in: %+v", lower.Refs)
	}
}

// TestRefsLocalIsNotADefinition: a local `Target := 2` is a use of the name. Calling it a
// definition would rank it above the call sites that were actually asked for.
func TestRefsLocalIsNotADefinition(t *testing.T) {
	scan, err := Refs([]string{refTree(t)}, -1, RefQuery{Symbol: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	refs := scan.Refs
	for _, r := range find(refs, "lib/lib.go", 21) { // Target := 2
		if r.Kind == RefDef {
			t.Errorf("a local declaration must not rank as a definition: %+v", r)
		}
	}
}

// TestRefsRanksDefsThenCallsThenTests: the ordering IS the feature — a file-ordered list is what
// grep already gives.
func TestRefsRanksDefsThenCallsThenTests(t *testing.T) {
	scan, err := Refs([]string{refTree(t)}, -1, RefQuery{Symbol: "Use"})
	if err != nil {
		t.Fatal(err)
	}
	refs := scan.Refs
	if len(refs) != 3 { // the definition, the production call, the test call
		t.Fatalf("expected 3 hits for Use, got %d: %+v", len(refs), refs)
	}
	if refs[0].Kind != RefDef {
		t.Errorf("the definition must lead, got %v (%s:%d)", refs[0].Kind, refs[0].Path, refs[0].Line)
	}
	if refs[1].Test || !refs[2].Test {
		t.Errorf("the production call must precede the test call, got %+v", refs[1:])
	}
	// Every non-test hit precedes every test hit of the same kind.
	lastNonTest, firstTest := -1, len(refs)
	for i, r := range refs {
		if r.Kind != RefCall {
			continue
		}
		if r.Test && i < firstTest {
			firstTest = i
		}
		if !r.Test && i > lastNonTest {
			lastNonTest = i
		}
	}
	if lastNonTest > firstTest {
		t.Errorf("a test call ranked above a production call:\n%+v", refs)
	}
}

// TestRefsCarriesContext: each hit names where you landed — the arch header's package and the
// enclosing declaration — which is the context brokkr already parses and grep cannot.
func TestRefsCarriesContext(t *testing.T) {
	scan, err := Refs([]string{refTree(t)}, -1, RefQuery{Symbol: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	refs := scan.Refs
	got := find(refs, "app/app.go", 12)
	if len(got) != 1 {
		t.Fatalf("want the selector call, got %d hits", len(got))
	}
	if got[0].Pkg != "app / app" {
		t.Errorf("Pkg = %q, want the arch header's package field %q", got[0].Pkg, "app / app")
	}
	if !strings.Contains(got[0].Encl, "Use") {
		t.Errorf("Encl = %q, want the enclosing func Use", got[0].Encl)
	}
	if got[0].Text == "" {
		t.Error("a hit must carry its source line")
	}
}

// TestRefsComments: prose is excluded by default (it is not a reference) and ranks last when asked
// for, so --comments can never push a real call site off the top.
func TestRefsComments(t *testing.T) {
	dir := refTree(t)
	for _, r := range mustRefs(t, dir, RefQuery{Symbol: "Target"}) {
		if r.Kind == RefComment {
			t.Errorf("comments must be off by default, got %s:%d", r.Path, r.Line)
		}
	}
	withProse := mustRefs(t, dir, RefQuery{Symbol: "Target", Comments: true})
	var seen bool
	for i, r := range withProse {
		if r.Kind != RefComment {
			continue
		}
		seen = true
		for _, later := range withProse[i:] {
			if later.Kind != RefComment {
				t.Errorf("a comment ranked above a %v hit", later.Kind)
			}
		}
	}
	if !seen {
		t.Error("--comments should have found the doc comments naming Target")
	}
}

// TestRefsRejectsAPattern: a regexp silently matching nothing reads as "unused", which is the
// wrong answer to the wrong question — so say which tool takes patterns.
func TestRefsRejectsAPattern(t *testing.T) {
	for _, bad := range []string{"", "Tar.*", "foo bar", "pkg.Foo", "3x"} {
		_, err := Refs([]string{t.TempDir()}, -1, RefQuery{Symbol: bad})
		if err == nil {
			t.Errorf("Refs(%q) should refuse a non-identifier", bad)
			continue
		}
		if !strings.Contains(err.Error(), "map --grep") {
			t.Errorf("the refusal should point at the pattern search, got: %v", err)
		}
	}
}

// TestWriteRefsReport covers the rendering: a tally, ranked sections, positions with columns, and
// the "nothing found" case explaining WHY it might be empty rather than just saying so.
func TestWriteRefsReport(t *testing.T) {
	dir := refTree(t)
	var sb strings.Builder
	if err := WriteRefs(&sb, []string{dir}, -1, RefQuery{Symbol: "Target"}, 0); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"Target:", "definitions", "── calls ──", "lib.go:14:", "« lib / lib"} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}

	sb.Reset()
	if err := WriteRefs(&sb, []string{dir}, -1, RefQuery{Symbol: "Absent"}, 0); err != nil {
		t.Fatal(err)
	}
	// An empty answer says what was READ, then why a miss is possible — the two halves of an
	// honest "nothing found".
	for _, want := range []string{"scanned ", " Go files", "no match for Absent", "case-sensitive"} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("an empty result must mention %q, got:\n%s", want, sb.String())
		}
	}
}

// TestWriteRefsLimitTrimsTheTail: the limit drops the LEAST relevant hits and admits how many,
// which is only safe because the report is ranked.
func TestWriteRefsLimitTrimsTheTail(t *testing.T) {
	var sb strings.Builder
	if err := WriteRefs(&sb, []string{refTree(t)}, -1, RefQuery{Symbol: "Target"}, 2); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "withheld") {
		t.Errorf("a truncated report must say how many were withheld:\n%s", out)
	}
	if !strings.Contains(out, "definitions") {
		t.Errorf("the kept hits must be the most relevant ones:\n%s", out)
	}
}

// TestRefsBadRoot fails loud: a typo'd path that reported "no references" would read as an answer.
func TestRefsBadRoot(t *testing.T) {
	if _, err := Refs([]string{filepath.Join(t.TempDir(), "nope")}, -1, RefQuery{Symbol: "Target"}); err == nil {
		t.Error("an unreadable root must be an error, not an empty answer")
	}
}

// mustRefs is Refs or a fatal — the tests below care about the results, not the plumbing.
func mustRefs(t *testing.T, dir string, q RefQuery) []Ref {
	t.Helper()
	scan, err := Refs([]string{dir}, -1, q)
	if err != nil {
		t.Fatal(err)
	}
	return scan.Refs
}
