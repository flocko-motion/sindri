// package: ui / importguard
// type:    test (architecture invariant)
// job:     fail the build if any package under internal/ui imports internal/hub or
// any of its subpackages — the front-ends reach the hub only through
// internal/api (the exchange format) and internal/client (the wire client).
// limits:  this one invariant only; it says nothing about what internal/ui may
// import besides the core.
package ui

import (
	"os/exec"
	"strings"
	"testing"

	// Blank-imported so this test's go-test cache key includes their transitive source —
	// go list -deps below is a subprocess Go's cache can't see into on its own, so without
	// these edges a change under one of them would leave a stale PASS cached here.
	_ "github.com/flo-at/sindri/internal/ui/attach"
	_ "github.com/flo-at/sindri/internal/ui/cli"
	_ "github.com/flo-at/sindri/internal/ui/theme"
	_ "github.com/flo-at/sindri/internal/ui/tui"
)

// TestFrontEndsDoNotImportHub walks the real import graph (via `go list -deps`, so it
// catches an indirect import too — one internal/ui package pulling in internal/hub
// through another) rather than grepping source, which a re-export could dodge.
func TestFrontEndsDoNotImportHub(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "./...").Output()
	if err != nil {
		t.Fatalf("go list -deps ./...: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "github.com/flo-at/sindri/internal/hub" || strings.HasPrefix(line, "github.com/flo-at/sindri/internal/hub/") {
			t.Errorf("internal/ui depends on %s — front-ends reach the hub only through internal/api and internal/client", line)
		}
	}
}
