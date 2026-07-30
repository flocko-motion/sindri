package codemap

import (
	"path/filepath"
	"strings"
	"testing"
)

// searchTree is a file whose query hits deliberately land in the three places the old
// single --grep handled badly: inside a func (fine), inside a package-level var (the
// declaration was invisible — you got the callers instead), and in the arch header
// only (the file rendered with no declarations at all, a match you couldn't locate).
const searchSrc = `// package: pkg / a
// type:    logic
// job:     mind the widget budget
// limits:  none
package pkg

// budget is the widget ceiling.
var budget = 10

// Spend draws down the budget.
func Spend(n int) int { return budget - n }

// Idle does nothing with it.
func Idle() {}
`

func searchTree(t *testing.T) string {
	t.Helper()
	return filepath.Join(writeTree(t, map[string]string{"pkg/a.go": searchSrc}), "pkg")
}

// TestFindShowsMatchingVarDecl: a query naming a package-level var must show the
// declaration, not just the functions that use it. Vars are absent from the plain map
// on purpose, but a search for one that answers with everything EXCEPT it is a trap.
func TestFindShowsMatchingVarDecl(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{searchTree(t)}, -1, Query{Find: "budget"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "var budget") {
		t.Errorf("--find should show the var declaration itself, got:\n%s", out)
	}
	if !strings.Contains(out, "func Spend") {
		t.Errorf("--find should still show the enclosing func, got:\n%s", out)
	}
	if strings.Contains(out, "func Idle") {
		t.Errorf("--find should omit non-matching decls, got:\n%s", out)
	}
}

// TestFindLocatesHeaderOnlyMatch: when the only match is in the arch header, the file
// used to print its header and nothing else — indistinguishable from "matched, but I
// won't tell you where". The matching line must be named.
func TestFindLocatesHeaderOnlyMatch(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{searchTree(t)}, -1, Query{Find: "widget ceiling|mind the widget"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "   3  // job:") {
		t.Errorf("--find should name the matching header line, got:\n%s", out)
	}
}

// TestFindSkipsNonMatchingFiles keeps the narrowing property: a file with no match is
// absent entirely, not present-but-empty.
func TestFindSkipsNonMatchingFiles(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{searchTree(t)}, -1, Query{Find: "nothinghere"}); err != nil {
		t.Fatal(err)
	}
	if out := b.String(); strings.TrimSpace(out) != "" {
		t.Errorf("a query with no matches should print nothing, got:\n%s", out)
	}
}

// TestGrepEmitsTaggedLines: the line search answers with locations — `path:line: text`
// so editor tooling can jump to it — each tagged with the declaration it sits in,
// which is the part plain grep can't tell you and the reason not to pipe through it.
func TestGrepEmitsTaggedLines(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{searchTree(t)}, -1, Query{Grep: "budget - n"}); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(b.String())
	if want := "a.go:11: func Spend(n int) int { return budget - n }  « func Spend"; out != want {
		t.Errorf("grep line\n got: %q\nwant: %q", out, want)
	}
}

// TestGrepTagsLineOutsideAnyDecl: a match in the arch header has no enclosing
// declaration, so it carries no tag rather than a wrong one.
func TestGrepTagsLineOutsideAnyDecl(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{searchTree(t)}, -1, Query{Grep: "mind the widget"}); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(b.String())
	if strings.Contains(out, "«") {
		t.Errorf("a header match sits in no decl and should carry no tag, got: %q", out)
	}
	if !strings.HasPrefix(out, "a.go:3: ") {
		t.Errorf("grep should still locate it, got: %q", out)
	}
}

// TestSmartCase: a lowercase pattern searches loosely (what the old substring --grep
// did), an uppercase letter makes it case-sensitive — so `Spend` doesn't drag in
// every "spend" in prose.
func TestSmartCase(t *testing.T) {
	dir := searchTree(t)
	for _, tc := range []struct {
		pat  string
		want bool
	}{
		{"spend", true},  // lowercase → insensitive → matches func Spend
		{"SPEND", false}, // has uppercase → sensitive → no match
		{"Spend", true},  // has uppercase → sensitive → matches exactly
	} {
		var b strings.Builder
		if err := Write(&b, []string{dir}, -1, Query{Grep: tc.pat}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(b.String(), "Spend"); got != tc.want {
			t.Errorf("grep %q matched=%v, want %v", tc.pat, got, tc.want)
		}
	}
}

// TestGrepIsRegex: --grep takes a pattern, not a substring — the flag has to behave
// the way its name promises.
func TestGrepIsRegex(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{searchTree(t)}, -1, Query{Grep: `^func (Spend|Idle)`}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "func Spend") || !strings.Contains(out, "func Idle") {
		t.Errorf("alternation should match both funcs, got:\n%s", out)
	}
}

// TestFindAndGrepAreExclusive: they answer different questions, so silently letting
// one win would hand back the wrong shape of answer.
func TestFindAndGrepAreExclusive(t *testing.T) {
	var b strings.Builder
	err := Write(&b, []string{searchTree(t)}, -1, Query{Find: "a", Grep: "b"})
	if err == nil || !strings.Contains(err.Error(), "pass one") {
		t.Fatalf("expected an exclusivity error, got %v", err)
	}
}

// TestBadPatternNamesTheFlag: a malformed regex must say which flag it came from.
func TestBadPatternNamesTheFlag(t *testing.T) {
	var b strings.Builder
	err := Write(&b, []string{searchTree(t)}, -1, Query{Find: "[unclosed"})
	if err == nil || !strings.Contains(err.Error(), "--find") {
		t.Fatalf("expected an error naming --find, got %v", err)
	}
}

// TestPlainMapOmitsValueDecls: collecting vars/consts is for searching only. The map
// itself must stay the high-signal types-and-funcs view it was.
func TestPlainMapOmitsValueDecls(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{searchTree(t)}, -1, Query{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "var budget") {
		t.Errorf("the plain map should not list package vars, got:\n%s", out)
	}
	if !strings.Contains(out, "func Spend") {
		t.Errorf("the plain map should still list funcs, got:\n%s", out)
	}
}

// symbolSrc exercises every shape --symbol has to get right: a grouped const block (the
// queried name isn't always the first, which a naive label-based lookup would miss), two
// methods sharing a bare name across different receivers, and a plain func for the exact-
// match/case-sensitivity tests.
const symbolSrc = `// package: pkg / b
// type:    logic
// job:     exercise --symbol
// limits:  none
package pkg

const (
	// first is the earlier of the pair.
	first = 1
	// second is the one a first-name-only lookup would miss.
	second = 2
)

// Widget is a thing.
type Widget struct{}

// Open opens a Widget.
func (w *Widget) Open() {}

// Gadget is another thing, unrelated to Widget.
type Gadget struct{}

// Open opens a Gadget — same bare name as Widget's, a different receiver.
func (g *Gadget) Open() {}

// Spend is unrelated, for the exact-match and case tests.
func Spend(n int) int { return n }
`

func symbolTree(t *testing.T) string {
	t.Helper()
	return filepath.Join(writeTree(t, map[string]string{"pkg/b.go": symbolSrc}), "pkg")
}

// TestSymbolFindsExactFunc: the plain, common case — a bare func name finds its declaration.
func TestSymbolFindsExactFunc(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "Spend"}); err != nil {
		t.Fatal(err)
	}
	if out := b.String(); !strings.Contains(out, "func Spend") {
		t.Errorf("expected the Spend declaration, got:\n%s", out)
	}
}

// TestSymbolIsExactNotSubstring: unlike --find/--grep, --symbol is a lookup, not a text
// search — "Spen" must never match "Spend", or the flag's whole point (a name means exactly
// one thing) breaks.
func TestSymbolIsExactNotSubstring(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "Spen"}); err != nil {
		t.Fatal(err)
	}
	if out := strings.TrimSpace(b.String()); out != "" {
		t.Errorf("a partial name must not match, got:\n%s", out)
	}
}

// TestSymbolIsCaseSensitive: --find/--grep are smart-case for casual text search; --symbol
// is an identifier lookup, always case-sensitive, so it agrees with `brokkr refs` on what a
// name means.
func TestSymbolIsCaseSensitive(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "spend"}); err != nil {
		t.Fatal(err)
	}
	if out := strings.TrimSpace(b.String()); out != "" {
		t.Errorf("a lowercase query must not match Spend, got:\n%s", out)
	}
}

// TestSymbolFindsValueDecl: vars/consts are excluded from the plain map (they'd bury it) but
// must still be reachable by an exact lookup, the same exemption --find already gets.
func TestSymbolFindsValueDecl(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "first"}); err != nil {
		t.Fatal(err)
	}
	if out := b.String(); !strings.Contains(out, "const first") {
		t.Errorf("expected the const declaration, got:\n%s", out)
	}
}

// TestSymbolFindsSecondNameInGroupedDecl: a grouped `const ( first; second )` is one
// declaration for two symbols, and its printed label only ever shows the first — a lookup
// that trusted the label instead of the real identifiers would silently never find "second".
func TestSymbolFindsSecondNameInGroupedDecl(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "second"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "const second") {
		t.Errorf("expected to find the second name in the group, got:\n%s", out)
	}
	// The whole block is one unit — its sibling comes along, which is the point: the group IS
	// the symbol's context.
	if !strings.Contains(out, "const first") {
		t.Errorf("the group's other constant should still be shown as context, got:\n%s", out)
	}
}

// TestSymbolMatchesEveryReceiver: a bare method name is queried without its receiver, so both
// Widget.Open and Gadget.Open must come back — the point of an exact-identifier lookup is
// finding every candidate, not guessing which one you meant.
func TestSymbolMatchesEveryReceiver(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "Open"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "func (w *Widget) Open()") {
		t.Errorf("expected Widget's Open, got:\n%s", out)
	}
	if !strings.Contains(out, "func (g *Gadget) Open()") {
		t.Errorf("expected Gadget's Open too, got:\n%s", out)
	}
}

// TestSymbolFindsType: a type name is as valid a lookup target as a func.
func TestSymbolFindsType(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "Gadget"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "type Gadget") {
		t.Errorf("expected the Gadget type, got:\n%s", out)
	}
	if strings.Contains(out, "type Widget") {
		t.Errorf("must not also show the unrelated Widget type, got:\n%s", out)
	}
}

// TestSymbolSkipsNonMatchingFiles: the narrowing property --find already has — a file with no
// match is absent entirely, not present-but-empty.
func TestSymbolSkipsNonMatchingFiles(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "NoSuchSymbol"}); err != nil {
		t.Fatal(err)
	}
	if out := strings.TrimSpace(b.String()); out != "" {
		t.Errorf("an unmatched symbol should print nothing, got:\n%s", out)
	}
}

// TestSymbolIsExclusiveWithFindAndGrep: three different searches, and mixing them would hand
// back the wrong shape of answer, so it errors rather than silently picking one.
func TestSymbolIsExclusiveWithFindAndGrep(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "Spend", Find: "a"}); err == nil || !strings.Contains(err.Error(), "pass one") {
		t.Fatalf("--symbol + --find: expected an exclusivity error, got %v", err)
	}
	if err := Write(&b, []string{symbolTree(t)}, -1, Query{Symbol: "Spend", Grep: "a"}); err == nil || !strings.Contains(err.Error(), "pass one") {
		t.Fatalf("--symbol + --grep: expected an exclusivity error, got %v", err)
	}
}

// TestGrepOverBudgetTruncates: headers-only is a meaningful reduction for a map and a
// non-answer for grep, so an over-budget line search keeps real matches and says how
// many it cut.
func TestGrepOverBudgetTruncates(t *testing.T) {
	var b strings.Builder
	if err := WriteAdaptive(&b, []string{searchTree(t)}, -1, Query{Grep: "e"}, false, 2); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "headers only") {
		t.Errorf("grep should not reduce to file headers, got:\n%s", out)
	}
	if !strings.Contains(out, "more matching lines") {
		t.Errorf("expected a truncation note, got:\n%s", out)
	}
	if !strings.Contains(out, "a.go:") {
		t.Errorf("truncated output should still carry matches, got:\n%s", out)
	}
}
