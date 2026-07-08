package hub

import (
	"strings"
	"testing"
)

// TestSystemPromptCarriesArchitecture: every role's durable brief points the agent at
// the project's architecture doc — not just the reviewer. An agent can't produce work
// that fits the project without knowing how it's built.
func TestSystemPromptCarriesArchitecture(t *testing.T) {
	const arch = "docs/ARCH.md"
	for _, role := range []string{"worker", "reviewer", "planner", "coauthor"} {
		p := systemPrompt("eitri", role, arch)
		if !strings.Contains(p, "/workspace/"+arch) {
			t.Errorf("%s system prompt does not tell the agent to read /workspace/%s:\n%s", role, arch, p)
		}
	}
}
