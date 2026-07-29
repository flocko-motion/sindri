package lint

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAllowanceMatchesSpec pins the rule to its formula: with n comment blocks under the trend
// sample, a file may average ((10-n)*0.5+1) × the configured maximum. A sparsely-commented file
// earns room for longer comments; a well-commented one answers for its trend.
func TestAllowanceMatchesSpec(t *testing.T) {
	const x = 2.0
	for n := 1; n <= 14; n++ {
		want := x
		if n < 10 {
			want = (float64(10-n)*0.5 + 1) * x
		}
		if got := allowanceFor(x, n); math.Abs(got-want) > 1e-9 {
			t.Errorf("n=%d: allowance %.2f, want %.2f", n, got, want)
		}
		fmt.Printf("  n=%2d  allowed mean = %5.2f lines\n", n, allowanceFor(x, n))
	}
}

// TestAllowanceIsMonotonic: more comments never buys more slack, so a file cannot game the rule
// by adding one-liners to raise its own ceiling.
func TestAllowanceIsMonotonic(t *testing.T) {
	prev := math.Inf(1)
	for n := 1; n <= 20; n++ {
		got := allowanceFor(2.0, n)
		if got > prev {
			t.Errorf("n=%d: allowance rose to %.2f from %.2f", n, got, prev)
		}
		prev = got
	}
}

// writeTree lays out a fixture tree and returns its root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// tsHeader is a valid four-field header followed by an import, as real source has: CODE is
// what closes a comment block, so the header stands alone only when something follows it.
const tsHeader = "// package: ui / thing\n// type:    ui\n// job:     do the thing\n// limits:  nothing else\nimport {x} from './x';\n\n"

// TestCommentAvgCoversTypeScript: the rule reads the TypeScript/JavaScript family too, React
// components included — one house style across the repo rather than one per language.
func TestCommentAvgCoversTypeScript(t *testing.T) {
	long := ""
	for i := 0; i < 10; i++ { // ten blocks: the sample is full, so the configured maximum applies
		long += fmt.Sprintf("// line one of comment %d\n// line two\n// line three\nexport const v%d = %d;\n\n", i, i, i)
	}
	root := writeTree(t, map[string]string{
		"src/Widget.tsx": tsHeader + long,
		"src/Fine.tsx":   tsHeader + "// short\nexport const a = 1;\n\n// also short\nexport const b = 2;\n",
	})

	var out bytes.Buffer
	found, err := CommentAvg([]string{root}, 2.0, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("expected the long-comment .tsx to be reported, got:\n%s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "Widget.tsx") {
		t.Errorf("the offending .tsx should be named:\n%s", got)
	}
	if strings.Contains(got, "Fine.tsx") {
		t.Errorf("a file within the trend must not be reported:\n%s", got)
	}
}

// TestCommentAvgExcludesHeaderAndTests: the header is a REQUIRED multi-line block, so counting
// it would charge every file for obeying the header rule; tests carry their own conventions.
func TestCommentAvgExcludesHeaderAndTests(t *testing.T) {
	// A file whose ONLY multi-line comment is its header, with terse comments after it.
	body := tsHeader
	for i := 0; i < 12; i++ {
		body += fmt.Sprintf("// note %d\nexport const v%d = %d;\n", i, i, i)
	}
	root := writeTree(t, map[string]string{
		"src/Headered.tsx":       body,
		"src/thing.test.ts":      "// one\n// two\n// three\n// four\n// five\nexport const x = 1;\n",
		"src/__tests__/spec.tsx": "// one\n// two\n// three\n// four\n// five\nexport const y = 1;\n",
	})

	var out bytes.Buffer
	found, err := CommentAvg([]string{root}, 2.0, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("header and test files must not trip the rule:\n%s", out.String())
	}
}

// TestCommentAvgThinSampleIsForgiven: two long comments are not a trend, and the report says
// which allowance applied so the verdict is checkable rather than mysterious.
func TestCommentAvgThinSampleIsForgiven(t *testing.T) {
	eight := "// a\n// b\n// c\n// d\n// e\n// f\n// g\n// h\n"
	root := writeTree(t, map[string]string{
		// Two 8-line comments → mean 8.0, allowance at n=2 is (1+0.5*8)*2.0 = 10.0.
		"src/thin.tsx": tsHeader + eight + "export const a = 1;\n\n" + eight + "export const b = 2;\n",
	})
	var out bytes.Buffer
	found, err := CommentAvg([]string{root}, 2.0, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("a two-comment file should be forgiven at 2.0 (allowance 10.0):\n%s", out.String())
	}
}

// TestBlankLinesCannotSplitAComment: a long comment broken up by blank lines is still one long
// comment. If a gap ended a block, any file could halve its measured comment length by adding
// them, and the rule would measure formatting rather than prose.
func TestBlankLinesCannotSplitAComment(t *testing.T) {
	const gapped = "// part one line a\n// part one line b\n\n// part two line a\n// part two line b\n"
	blocks := ScanComments(gapped)
	if len(blocks) != 1 {
		t.Fatalf("a gapped comment must scan as one block, got %d", len(blocks))
	}
	if blocks[0].Lines != 4 {
		t.Errorf("block should count its 4 comment lines (not the blank), got %d", blocks[0].Lines)
	}
	// Code, by contrast, does end a block.
	if got := ScanComments("// one\ncode()\n// two\n"); len(got) != 2 {
		t.Errorf("code between comments must split them, got %d blocks", len(got))
	}
}

// TestDelimitersAreFormattingNotProse: the rule keeps comments SHORT, so it must let them be
// formatted properly. `/**` and `*/` on their own lines are formatting; counting them charged
// JSDoc two lines that Go's `//` never pays, so the same prose failed in TypeScript and passed
// in Go. A bare `*` inside the block is a paragraph break the author wrote, and still counts.
func TestDelimitersAreFormattingNotProse(t *testing.T) {
	jsdoc := "/**\n * one\n *\n * two\n */\ncode()\n"
	golike := "// one\n//\n// two\ncode()\n"

	js, slashes := ScanComments(jsdoc), ScanComments(golike)
	if len(js) != 1 || len(slashes) != 1 {
		t.Fatalf("each source is one block, got %d and %d", len(js), len(slashes))
	}
	if js[0].Lines != slashes[0].Lines {
		t.Errorf("identical prose must measure the same: JSDoc %d lines, Go %d", js[0].Lines, slashes[0].Lines)
	}
	if js[0].Lines != 3 {
		t.Errorf("three content lines (one, blank, two), got %d", js[0].Lines)
	}
	// The block is still reported at the line it opens on, not at its first prose line.
	if js[0].Line != 1 {
		t.Errorf("block should start at the `/**` line 1, got %d", js[0].Line)
	}
	// A one-line block comment carries its prose; a block of pure delimiters is not a comment.
	if got := ScanComments("/** just this */\ncode()\n"); len(got) != 1 || got[0].Lines != 1 {
		t.Errorf("a one-line /** … */ is one line, got %+v", got)
	}
	if got := ScanComments("/**\n */\ncode()\n"); len(got) != 0 {
		t.Errorf("delimiters alone are not a comment, got %+v", got)
	}
}

// TestExcerptNamesTheComment: the report's excerpt must quote the offending comment. It used to
// print the `*` left over from `/**`, which named nothing — worst for TypeScript, where every
// block comment opens that way.
func TestExcerptNamesTheComment(t *testing.T) {
	long := "/**\n * the offending explanation\n * b\n * c\n * d\n * e\n * f\n * g\n * h\n * i\n */\n"
	root := writeTree(t, map[string]string{"src/Wordy.tsx": tsHeader + long + "export const a = 1;\n"})

	// A deliberately strict maximum, so this test turns on the excerpt rather than on where the
	// thin-sample allowance happens to fall.
	var out bytes.Buffer
	found, err := CommentAvg([]string{root}, 0.5, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("a 9-line comment must trip a 0.5 maximum:\n%s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "the offending explanation") {
		t.Errorf("the excerpt should quote the comment:\n%s", got)
	}
	if strings.Contains(got, "    *…") {
		t.Errorf("the excerpt must not be a bare delimiter:\n%s", got)
	}
}

func mustIgnore(t *testing.T) *Ignore {
	t.Helper()
	ig, err := NewIgnore(nil)
	if err != nil {
		t.Fatal(err)
	}
	return ig
}
