package api

import (
	"go/build"
	"strings"
	"testing"
)

// TestImportsNothingInternal pins the invariant the whole exchange-package split rests
// on: the hub and every front-end can depend on internal/api without either dragging
// the other in, which holds only if this package imports nothing beyond the standard
// library. A stdlib import path never contains a dot before its first slash (e.g.
// "encoding/json"); anything else (e.g. "github.com/...") does.
func TestImportsNothingInternal(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range pkg.Imports {
		if first := strings.SplitN(imp, "/", 2)[0]; strings.Contains(first, ".") {
			t.Errorf("internal/api must import only the standard library, found %q", imp)
		}
	}
}
