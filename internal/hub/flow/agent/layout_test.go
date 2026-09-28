package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryLayoutNamesOnlyItsOwnStates: a layout is exported once and then outlives edits to the
// flow, so a state renamed or removed leaves a position for nothing. That fails here, with the name;
// a state the layout lacks does not — the view places it and the next export picks it up.
func TestEveryLayoutNamesOnlyItsOwnStates(t *testing.T) {
	for _, role := range Roles {
		declared := map[string]bool{}
		for _, s := range Of(role) {
			declared[s.Name] = true
		}
		for name := range LayoutOf(role) {
			if !declared[name] {
				t.Errorf("%s's layout places %q, which its flow does not declare — re-export the layout", role, name)
			}
		}
	}
}

// TestTheExportNamesARealLayoutFile: the export tells a developer which file to replace, so that
// file must exist and declare the Layout the registry reads.
func TestTheExportNamesARealLayoutFile(t *testing.T) {
	for _, role := range Roles {
		path, pkg := LayoutFile(role)
		src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", path))
		if err != nil {
			t.Errorf("%s: the export names %s, which cannot be read: %v", role, path, err)
			continue
		}
		if !strings.Contains(string(src), "package "+pkg+"\n") || !strings.Contains(string(src), "var Layout = machine.Layout{") {
			t.Errorf("%s: %s does not declare package %s with var Layout = machine.Layout{…}", role, path, pkg)
		}
	}
}
