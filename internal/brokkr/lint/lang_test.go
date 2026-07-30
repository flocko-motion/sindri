package lint

import (
	"reflect"
	"testing"
)

// TestSplitHeaderSeparatesTrailingProse: a header block is the four fields and their wrapped
// continuations; a blank comment line after them ends it, and everything past that gap is an
// ordinary comment — not header, and not exempt from the length rules.
func TestSplitHeaderSeparatesTrailingProse(t *testing.T) {
	src := "// package: p / q\n" +
		"// type:    logic\n" +
		"// job:     does the thing\n" +
		"// limits:  nothing else\n" +
		"//\n" +
		"// A longer explanation that has no business inflating the header's own exemption from\n" +
		"// the comment-length trend, since it is not one of the four fields at all.\n" +
		"package p\n"
	bs := ScanComments(src)
	header, ok := HeaderBlock(bs)
	if !ok {
		t.Fatal("expected a header block")
	}
	fields, prose, hasProse := SplitHeader(header)
	if !hasProse {
		t.Fatal("expected trailing prose to be split out")
	}
	if fields.Lines != 4 {
		t.Errorf("fields.Lines = %d, want 4 (package/type/job/limits only)", fields.Lines)
	}
	if prose.Lines != 2 {
		t.Errorf("prose.Lines = %d, want 2", prose.Lines)
	}
	wantText := []string{
		"A longer explanation that has no business inflating the header's own exemption from",
		"the comment-length trend, since it is not one of the four fields at all.",
	}
	if !reflect.DeepEqual(prose.Text, wantText) {
		t.Errorf("prose.Text = %q, want %q", prose.Text, wantText)
	}
	// The split lines must point at their REAL source lines, so a report on the split-out prose
	// still names where to look rather than the header's own opening line.
	if prose.Line != 6 || prose.End != 7 {
		t.Errorf("prose.Line/End = %d/%d, want 6/7", prose.Line, prose.End)
	}
}

// TestSplitHeaderWithoutTrailingProse: a header that is exactly the four fields, nothing after,
// must not manufacture a prose block out of thin air.
func TestSplitHeaderWithoutTrailingProse(t *testing.T) {
	src := "// package: p / q\n// type:    logic\n// job:     does the thing\n// limits:  nothing else\npackage p\n"
	header, ok := HeaderBlock(ScanComments(src))
	if !ok {
		t.Fatal("expected a header block")
	}
	fields, _, hasProse := SplitHeader(header)
	if hasProse {
		t.Error("a plain four-field header must not report trailing prose")
	}
	if fields.Lines != 4 {
		t.Errorf("fields.Lines = %d, want 4", fields.Lines)
	}
}

// TestSplitHeaderTrailingBlankIsNotProse: a header ending in a bare blank comment line — no
// paragraph after it — must not count that gap itself as smuggled prose.
func TestSplitHeaderTrailingBlankIsNotProse(t *testing.T) {
	src := "// package: p / q\n// type:    logic\n// job:     does the thing\n// limits:  nothing else\n//\npackage p\n"
	header, ok := HeaderBlock(ScanComments(src))
	if !ok {
		t.Fatal("expected a header block")
	}
	_, _, hasProse := SplitHeader(header)
	if hasProse {
		t.Error("a trailing blank line with nothing after it is not prose")
	}
}

// TestSplitHeaderWithNoFieldsIsUntouched: a file whose leading comment carries none of the four
// fields is not a header at all — SplitHeader must leave it whole for the missing-header check to
// reject, not silently classify part of it as "prose".
func TestSplitHeaderWithNoFieldsIsUntouched(t *testing.T) {
	src := "// just some prose above a package clause, no header fields in sight\npackage p\n"
	header, ok := HeaderBlock(ScanComments(src))
	if !ok {
		t.Fatal("expected a (malformed) leading comment block")
	}
	fields, _, hasProse := SplitHeader(header)
	if hasProse {
		t.Error("a block with no header fields must not be split")
	}
	if fields.Lines != header.Lines {
		t.Errorf("fields.Lines = %d, want the whole block (%d)", fields.Lines, header.Lines)
	}
}
