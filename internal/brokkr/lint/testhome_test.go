package lint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStub puts a minimal Go file in dir, creating what it needs.
func writeStub(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestATestFileMustNameSomethingBesideIt is the rule itself: a stem that names no source is a test
// left behind by code that moved, which is how a package comes to be mostly tests for subjects it
// no longer holds.
func TestATestFileMustNameSomethingBesideIt(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "kept.go")
	writeStub(t, dir, "kept_test.go")
	writeStub(t, dir, "orphan_test.go")

	var out bytes.Buffer
	found, err := TestHome([]string{dir}, &Cap{}, &Ignore{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("orphan_test.go names no source and went unreported")
	}
	if strings.Contains(out.String(), "kept_test.go") {
		t.Errorf("kept_test.go sits beside kept.go and must not be reported: %s", out.String())
	}
	if !strings.Contains(out.String(), "orphan_test.go") {
		t.Errorf("the finding should name the file: %s", out.String())
	}
}

// TestASplitTestFileNamesWhatItSplits: a long test file is cut by aspect, not by inventing a source
// file to justify the name — so foo_bar_test.go is at home beside foo.go.
func TestASplitTestFileNamesWhatItSplits(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "gitcmd.go")
	writeStub(t, dir, "gitcmd_rollback_test.go")
	writeStub(t, dir, "gitcmd_drop_deep_test.go")

	var out bytes.Buffer
	found, err := TestHome([]string{dir}, &Cap{}, &Ignore{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("a split of gitcmd.go must be at home beside it: %s", out.String())
	}
}

// TestAPackageOfNothingButTestsIsExempt: internal/arch is guards over the whole tree, so there is no
// source beside them to name and demanding one would ask for an empty file.
func TestAPackageOfNothingButTestsIsExempt(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "guard_test.go")
	writeStub(t, dir, "another_test.go")

	var out bytes.Buffer
	found, err := TestHome([]string{dir}, &Cap{}, &Ignore{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("a test-only package has nothing to name: %s", out.String())
	}
}

// TestANameGoSilentlySkipsIsReported is the bug that prompted the rule: clear_arm_test.go ends in
// _arm, which Go reads as GOARCH=arm, so it was compiled on no machine anybody ran — and the
// assertion inside it was wrong the whole time. Nothing says so; the file is simply skipped.
func TestANameGoSilentlySkipsIsReported(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "clear.go")
	writeStub(t, dir, "clear_arm_test.go") // at home by stem, and still never compiled

	var out bytes.Buffer
	found, err := TestHome([]string{dir}, &Cap{}, &Ignore{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("a filename Go reads as a build constraint went unreported")
	}
	if !strings.Contains(out.String(), "build constraint") {
		t.Errorf("the finding should say WHY the file never runs: %s", out.String())
	}
}

// TestAnOrdinarySuffixIsNotAPlatform: the rule reads the last segment only, so a test split by
// aspect keeps working — "rollback" is not an architecture.
func TestAnOrdinarySuffixIsNotAPlatform(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "gitcmd.go")
	writeStub(t, dir, "gitcmd_rollback_test.go")

	var out bytes.Buffer
	found, err := TestHome([]string{dir}, &Cap{}, &Ignore{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("an ordinary aspect suffix must not be read as a platform: %s", out.String())
	}
}
