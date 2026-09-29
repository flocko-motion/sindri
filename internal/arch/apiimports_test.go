package arch

import (
	"go/build"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheExchangePackageImportsNothingInternal pins the invariant the whole exchange-package split
// rests on: the hub and every front-end can depend on internal/api without either dragging the
// other in, which holds only if that package imports nothing beyond the standard library. A stdlib
// import path never contains a dot before its first slash ("encoding/json"); anything else does.
//
// Here rather than beside the package it guards: it asserts a property of internal/api as a WHOLE,
// so there is no source file in it the test belongs to (-> brokkr lint test-home).
func TestTheExchangePackageImportsNothingInternal(t *testing.T) {
	dir := filepath.Join(moduleRoot(t), "internal", "api")
	pkg, err := build.ImportDir(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Imports) < 3 {
		t.Fatalf("only %d imports read from internal/api — the guard is reading the wrong directory", len(pkg.Imports))
	}
	for _, imp := range pkg.Imports {
		if first := strings.SplitN(imp, "/", 2)[0]; strings.Contains(first, ".") {
			t.Errorf("internal/api must import only the standard library, found %q", imp)
		}
	}
}
