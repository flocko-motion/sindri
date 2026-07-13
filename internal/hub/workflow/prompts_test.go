package workflow

import (
	"strings"
	"testing"
)

// TestSystemPromptCarriesArchitecture: every role's durable brief has the project
// architecture INJECTED (full content, not just a path) plus a re-read pointer — not
// just the reviewer. An agent can't produce work that fits without knowing how it's
// built. Empty content injects nothing.
func TestSystemPromptCarriesArchitecture(t *testing.T) {
	const archPath = "docs/ARCH.md"
	const archContent = "## Layering\nAdapters never import the hub."
	for _, role := range []string{"worker", "reviewer", "planner", "coauthor"} {
		p := SystemPrompt("eitri", role, archContent, archPath)
		if !strings.Contains(p, archContent) {
			t.Errorf("%s: architecture content not injected:\n%s", role, p)
		}
		if !strings.Contains(p, "/workspace/"+archPath) {
			t.Errorf("%s: missing re-read pointer to /workspace/%s", role, archPath)
		}
		if !strings.Contains(p, "`brokkr`") {
			t.Errorf("%s: brief should always recommend brokkr:\n%s", role, p)
		}
	}
	if p := SystemPrompt("eitri", "worker", "", "ARCHITECTURE.md"); strings.Contains(p, "Project architecture") {
		t.Errorf("empty architecture content should inject no section:\n%s", p)
	}
}
