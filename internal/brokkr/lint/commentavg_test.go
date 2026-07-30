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
	found, err := CommentAvg([]string{root}, 2.0, 0, false, nil, mustIgnore(t), &out)
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
	found, err := CommentAvg([]string{root}, 2.0, 0, false, nil, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("header and test files must not trip the rule:\n%s", out.String())
	}
}

// smuggledHeader is a valid four-field header followed by a blank comment line and a paragraph
// that carries none of the four fields — the shape reported as a cheat: park a long explanation
// above `package` and the header's own exemption used to hide it from the trend for free.
const smuggledHeader = "// package: seq / coordination\n" +
	"// type:    factory\n" +
	"// job:     build the backend named in config\n" +
	"// limits:  wiring only\n" +
	"//\n" +
	"// The single writer advances the head and keeps past heads for rollback. That mechanism\n" +
	"// belongs entirely to the backend: the merge steps, the write barrier, and the retained\n" +
	"// history a rollback reads from. This package only resolves a name to a constructor.\n" +
	"// Nothing about the sequencing logic itself lives here, by design, deliberately so.\n" +
	"import {x} from './x';\n\n"

// TestCommentAvgMeasuresProseSmuggledIntoTheHeader: a paragraph parked below the four fields, past
// a blank comment line, must count toward the trend like any other comment — otherwise the
// header's mandated exemption becomes a place to hide an unlimited explanation for free.
//
// Same body (nine 2-line comments) in both variants: honest-header passes at the generous n=9
// allowance (avg 2.0 <= 3.0); with the paragraph counted as a tenth block, the sample crosses into
// the tight n=10 allowance and the extra lines both push the average over it (avg 2.6 > 2.0).
func TestCommentAvgMeasuresProseSmuggledIntoTheHeader(t *testing.T) {
	nineTwoLiners := ""
	for i := 0; i < 9; i++ {
		nineTwoLiners += fmt.Sprintf("// note %d\n// still %d\nexport const v%d = %d;\n", i, i, i, i)
	}
	honest := writeTree(t, map[string]string{"src/Honest.tsx": tsHeader + nineTwoLiners})
	cheating := writeTree(t, map[string]string{"src/Cheating.tsx": smuggledHeader + nineTwoLiners})

	var honestOut, cheatingOut bytes.Buffer
	if found, err := CommentAvg([]string{honest}, 2.0, 0, false, nil, mustIgnore(t), &honestOut); err != nil {
		t.Fatal(err)
	} else if found {
		t.Errorf("the honest header (no smuggled paragraph) must not trip the rule:\n%s", honestOut.String())
	}

	found, err := CommentAvg([]string{cheating}, 2.0, 0, true, nil, mustIgnore(t), &cheatingOut)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("a paragraph smuggled below the header fields must trip the rule; it did not:\n%s", cheatingOut.String())
	}
	if !strings.Contains(cheatingOut.String(), "single writer advances") {
		t.Errorf("the report should name the smuggled paragraph as what to cut:\n%s", cheatingOut.String())
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
	found, err := CommentAvg([]string{root}, 2.0, 0, false, nil, mustIgnore(t), &out)
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

// TestGoDirectivesAreNotProse: //go:build and friends instruct the toolchain, so a file must not
// be charged for needing them. A comment that merely mentions one (with a space) is still prose.
func TestGoDirectivesAreNotProse(t *testing.T) {
	blocks := ScanComments("//go:build linux\n//go:generate stringer -type=T\n// the real explanation\ncode()\n")
	if len(blocks) != 1 {
		t.Fatalf("one block, got %d", len(blocks))
	}
	if blocks[0].Lines != 1 {
		t.Errorf("only the prose line counts, got %d", blocks[0].Lines)
	}
	if len(blocks[0].Text) != 1 || blocks[0].Text[0] != "the real explanation" {
		t.Errorf("the excerpt must be the prose, got %q", blocks[0].Text)
	}
	// A block of nothing but directives is not a comment at all.
	if got := ScanComments("//go:build linux\n\npackage x\n"); len(got) != 0 {
		t.Errorf("directives alone are not a comment, got %+v", got)
	}
	// "// go:build" — with a space — is prose, not a directive.
	if got := ScanComments("// go:build is how you gate a file\ncode()\n"); len(got) != 1 || got[0].Lines != 1 {
		t.Errorf("a comment mentioning a directive is still prose, got %+v", got)
	}
}

// TestBlocksListsEveryOffender: --blocks exists so fixing a file is one edit rather than a
// read-guess-recheck loop, which means naming EVERY comment over the limit with the range to go
// edit, and how many lines have to go. The range is physical (Line-End), not the prose count.
func TestBlocksListsEveryOffender(t *testing.T) {
	// Ten blocks, so the configured maximum applies exactly: three long, seven one-liners.
	body := tsHeader
	for i := 0; i < 3; i++ {
		body += fmt.Sprintf("/**\n * long comment %d needs cutting\n * second line\n * third line\n */\nexport const w%d = %d;\n\n", i, i, i)
	}
	for i := 0; i < 7; i++ {
		body += fmt.Sprintf("// terse %d\nexport const v%d = %d;\n\n", i, i, i)
	}
	root := writeTree(t, map[string]string{"src/Mixed.tsx": body})

	var out bytes.Buffer
	found, err := CommentAvg([]string{root}, 1.0, 0, true, nil, mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("expected a violation:\n%s", out.String())
	}
	got := out.String()
	// Every long block is listed, not just the longest.
	for i := 0; i < 3; i++ {
		if !strings.Contains(got, fmt.Sprintf("long comment %d", i)) {
			t.Errorf("block %d missing from the --blocks listing:\n%s", i, got)
		}
	}
	if !strings.Contains(got, "block(s) run over") {
		t.Errorf("the listing must say how many lines to cut:\n%s", got)
	}
	// A range, not a bare start line — the physical span is what you go and edit.
	if !strings.Contains(got, "-") || !strings.Contains(got, " lines  ") {
		t.Errorf("each block needs a line range and length:\n%s", got)
	}

	// Without --blocks the report stays the one-line summary it always was.
	var plain bytes.Buffer
	if _, err := CommentAvg([]string{root}, 1.0, 0, false, nil, mustIgnore(t), &plain); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "block(s) run over") {
		t.Errorf("the block listing must be opt-in:\n%s", plain.String())
	}
}

// TestBacktickCodeDoesNotHideComments guards a regression that was shipped and reverted: tracking
// raw strings by backtick parity to stop counting examples inside them. Backtick-heavy code (SQL
// spanning lines) flipped the state and left 27 real comment lines in one file UNMEASURED — the
// gate quietly weakened. Over-counting an example is the safer error; never stop counting.
func TestBacktickCodeDoesNotHideComments(t *testing.T) {
	src := "q := `SELECT a\nFROM b WHERE c = `+x+`\nORDER BY d`\n// a real comment below backtick-heavy code\ncode()\n"
	blocks := ScanComments(src)
	if len(blocks) != 1 {
		t.Fatalf("the comment must still be seen, got %d blocks: %+v", len(blocks), blocks)
	}
	if blocks[0].Lines != 1 {
		t.Errorf("one comment line, got %d", blocks[0].Lines)
	}
	// Backticks are everywhere in this repo's own comments; they must not change what follows.
	quoted := ScanComments("// use `sindri` for this\n// and `brokkr`\ncode()\n// after\ncode()\n")
	if len(quoted) != 2 {
		t.Errorf("quoted words must not swallow later comments, got %+v", quoted)
	}
}

// TestExcerptNamesTheComment: the report's excerpt must quote the offending comment. It used to
// print the `*` left over from `/**`, which named nothing — worst for TypeScript, where every
// block comment opens that way.
func TestExcerptNamesTheComment(t *testing.T) {
	long := "/**\n * the offending explanation\n * b\n * c\n * d\n * e\n * f\n * g\n * h\n * i\n */\n"
	root := writeTree(t, map[string]string{"src/Wordy.tsx": tsHeader + long + "export const a = 1;\n"})

	// blocks=true: the excerpt lives in the detail listing now. The default line carries line
	// RANGES instead — you open the file either way, so truncated prose found nothing for you.
	var out bytes.Buffer
	found, err := CommentAvg([]string{root}, 0.5, 0, true, nil, mustIgnore(t), &out)
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

// TestDefaultLineNamesLinesNotProse: the default report is one line per file, and what makes it
// actionable is the line RANGES to go and edit. It used to print a truncated excerpt, which told you
// nothing you could act on.
func TestDefaultLineNamesLinesNotProse(t *testing.T) {
	long := "/**\n * the offending explanation\n * b\n * c\n * d\n */\n"
	root := writeTree(t, map[string]string{"src/Wordy.tsx": tsHeader + long + "export const a = 1;\n"})

	var out bytes.Buffer
	if _, err := CommentAvg([]string{root}, 0.5, 0, false, nil, mustIgnore(t), &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "fix :") {
		t.Errorf("the default line must name the ranges to edit:\n%s", got)
	}
	if !strings.Contains(got, "target") {
		t.Errorf("the default line must measure against the target, not just the ceiling:\n%s", got)
	}
	if strings.Contains(got, "the offending explanation") {
		t.Errorf("no excerpt in the default output — that is what --blocks is for:\n%s", got)
	}
}

// TestReportAsksForTheJudgementNotJustTheNumber: reporting "ideal 1.5, max 2.0" let every reader
// treat the band as spare room — trim to 1.9, declare the bar cleared, done. Whether prose earns
// its length is not something a mean can decide, so the summary has to name the choice, say the
// check cannot make it, and ask for the reason. Wording is free; those three obligations are not.
func TestReportAsksForTheJudgementNotJustTheNumber(t *testing.T) {
	// Past trendSample blocks, so the small-file bonus is gone and the real 2.0 ceiling applies —
	// only then do the default target and ceiling appear as themselves.
	var src strings.Builder
	src.WriteString(tsHeader)
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&src, "// explanation %d\n// continued\n// and on\nexport const a%d = %d;\n\n", i, i, i)
	}
	root := writeTree(t, map[string]string{"src/Wordy.tsx": src.String()})

	var out bytes.Buffer
	if _, err := CommentAvg([]string{root}, DefaultMaxCommentAvg, 0, false, nil, mustIgnore(t), &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	// Both ends of the band, so the reader knows what is being chosen between.
	for _, want := range []string{"1.5", "2.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("the summary must name %s so the choice is visible:\n%s", want, got)
		}
	}
	// That the band is a claim about the prose, not permission to stop early.
	if !strings.Contains(got, "earn their length") {
		t.Errorf("the summary must say the band is a claim that the prose earns its length:\n%s", got)
	}
	// That the check disclaims the judgement rather than implying the number settled it.
	if !strings.Contains(got, "cannot judge") {
		t.Errorf("the summary must admit it cannot make this judgement:\n%s", got)
	}
	// And that a reason is owed — the only thing that separates valuable prose from padding.
	if !strings.Contains(got, "why") {
		t.Errorf("the summary must ask why, not only how much:\n%s", got)
	}
}

// TestAimNeverExceedsTheMax: the ideal is the max less a margin, but a max already under one line
// leaves no room — reporting "ideal 1.0, max 0.5" would ask for something the rule forbids.
func TestAimNeverExceedsTheMax(t *testing.T) {
	for _, max := range []float64{0.2, 0.5, 1.0, 1.5, 2.0, 8.0} {
		if aim := aimFor(max); aim > max {
			t.Errorf("aimFor(%.1f) = %.1f, must not exceed the max", max, aim)
		}
	}
	if aim := aimFor(2.0); aim != 1.5 {
		t.Errorf("the default max should give an ideal of 1.5, got %.1f", aim)
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
