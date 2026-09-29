package main

import (
	"reflect"
	"testing"
)

// TestSplitLintArgs: a first argument that names a linter selects it, anything else is a path. That
// is what lets `lint <file>` mean "every linter, this file" without a placeholder for the name.
func TestSplitLintArgs(t *testing.T) {
	for _, tc := range []struct {
		args      []string
		wantWhich string
		wantPaths []string
	}{
		{nil, "", nil},
		{[]string{"comment-length"}, "comment-length", []string{}},
		{[]string{"comment-length", "internal/hub/state.go"}, "comment-length", []string{"internal/hub/state.go"}},
		{[]string{"internal/hub"}, "", []string{"internal/hub"}},
		{[]string{"a.go", "b.go"}, "", []string{"a.go", "b.go"}},
		// A path that merely looks like a name is still dispatched as the linter — the names are
		// reserved words here, and no real file is called "loc".
		{[]string{"loc", "internal"}, "loc", []string{"internal"}},
	} {
		which, paths := splitLintArgs(tc.args)
		if which != tc.wantWhich {
			t.Errorf("%v: linter %q, want %q", tc.args, which, tc.wantWhich)
		}
		if len(paths) != len(tc.wantPaths) {
			t.Errorf("%v: paths %v, want %v", tc.args, paths, tc.wantPaths)
		}
	}
}

// TestPkgPatterns: deadcode analyses packages, so a scoped path has to become a package pattern —
// a file scoping to its directory, since a single file is not a unit it can load.
func TestPkgPatterns(t *testing.T) {
	if got := pkgPatterns(nil); !reflect.DeepEqual(got, []string{"./..."}) {
		t.Errorf("unscoped must stay ./..., got %v", got)
	}
	got := pkgPatterns([]string{"internal/hub/state.go", "internal/ui"})
	want := []string{"./internal/hub/...", "./internal/ui/..."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pkgPatterns = %v, want %v", got, want)
	}
}

// TestOrDot: an unscoped run must keep linting the whole tree, so the empty list means ".".
func TestOrDot(t *testing.T) {
	if got := orDot(nil); !reflect.DeepEqual(got, []string{"."}) {
		t.Errorf("orDot(nil) = %v, want [.]", got)
	}
	if got := orDot([]string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("orDot must pass scoped paths through, got %v", got)
	}
}
