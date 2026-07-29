package lint

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestCapBoundsOutputAndSaysSo: the point is that `brokkr lint` is safe to run WITHOUT --tail, so a
// run with hundreds of findings has to stop and say what it withheld. Silence about the rest would
// read as a clean tail.
func TestCapBoundsOutputAndSaysSo(t *testing.T) {
	c := NewCap(3)
	shown := 0
	for i := 0; i < 10; i++ {
		if c.Allow() {
			shown++
		}
	}
	if shown != 3 {
		t.Errorf("printed %d findings, want 3", shown)
	}
	if c.Hidden() != 7 {
		t.Errorf("withheld %d, want 7", c.Hidden())
	}
	var out bytes.Buffer
	c.Note(&out)
	if !strings.Contains(out.String(), "7 more") {
		t.Errorf("the note must count what was withheld: %q", out.String())
	}
	if !strings.Contains(out.String(), "--limit") {
		t.Errorf("the note must say how to see the rest: %q", out.String())
	}
}

// TestCapZeroAndNilPrintEverything: --limit 0 is the escape hatch, and a nil Cap lets a caller with
// no budget skip the special case — both must let every finding through and add no note.
func TestCapZeroAndNilPrintEverything(t *testing.T) {
	for _, c := range []*Cap{NewCap(0), nil} {
		for i := 0; i < 50; i++ {
			if !c.Allow() {
				t.Fatalf("%v should allow every finding", c)
			}
		}
		var out bytes.Buffer
		c.Note(&out)
		if out.Len() != 0 {
			t.Errorf("nothing withheld, so no note: %q", out.String())
		}
	}
}

// TestCapIsSharedAcrossLinters: the budget belongs to the RUN. Six linters each printing their own
// allowance is the wall it exists to prevent, so a later linter inherits what an earlier one spent.
func TestCapIsSharedAcrossLinters(t *testing.T) {
	c := NewCap(4)
	first, second := 0, 0
	for i := 0; i < 3; i++ { // first linter spends 3 of 4
		if c.Allow() {
			first++
		}
	}
	for i := 0; i < 3; i++ { // second gets the 1 that is left
		if c.Allow() {
			second++
		}
	}
	if first != 3 || second != 1 {
		t.Errorf("shared budget: first=%d second=%d, want 3 and 1", first, second)
	}
}

// TestCommentAvgHonoursTheCap wires it end to end: many failing files, few printed, and the count
// of files over the trend still reports the TRUE total rather than the number shown.
func TestCommentAvgHonoursTheCap(t *testing.T) {
	files := map[string]string{}
	long := ""
	for i := 0; i < 10; i++ {
		long += fmt.Sprintf("// line one of comment %d\n// line two\n// line three\nexport const v%d = %d;\n\n", i, i, i)
	}
	for f := 0; f < 8; f++ {
		files[fmt.Sprintf("src/F%d.tsx", f)] = tsHeader + long
	}
	root := writeTree(t, files)

	var out bytes.Buffer
	found, err := CommentAvg([]string{root}, 2.0, false, NewCap(3), mustIgnore(t), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("expected findings:\n%s", out.String())
	}
	got := out.String()
	if n := strings.Count(got, ".tsx: comments average"); n != 3 {
		t.Errorf("printed %d files, want 3:\n%s", n, got)
	}
	if !strings.Contains(got, "5 more") {
		t.Errorf("the 5 withheld files must be counted:\n%s", got)
	}
	if !strings.Contains(got, "8 file(s) over") {
		t.Errorf("the total must stay honest at 8, not the 3 shown:\n%s", got)
	}
}
