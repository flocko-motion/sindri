package cli

import (
	"strings"
	"testing"
)

// TestAgentNewStartsWhatItCreated pins the resolved divergence: the TUI launched a new agent and the
// CLI did not, so the same intent produced two behaviours. Both start now, and --no-start is the way
// back to a bare identity — which the hub spec requires stay possible ("an agent MAY exist with no
// pod: pre-declared, stopped, or crashed").
//
// This checks the command's surface, not a real launch: the CLI reaches the hub through a package
// function with no seam to inject a fake backend, so the launch itself is covered by the TUI's path
// and by using it. What it does catch is the surface silently reverting to register-only.
func TestAgentNewStartsWhatItCreated(t *testing.T) {
	c := agentNewCmd()
	if c.Flags().Lookup("no-start") == nil {
		t.Error("--no-start is missing — there is no way to pre-declare an agent without starting it")
	}
	// The old help promised the opposite of what the command now does.
	if strings.Contains(c.Short, "no container") {
		t.Errorf("the help still says the command creates no container: %q", c.Short)
	}
	if !strings.Contains(strings.ToLower(c.Short), "start") {
		t.Errorf("the help does not say the agent is started: %q", c.Short)
	}
	if !strings.Contains(c.Long, "--no-start") {
		t.Errorf("the long help does not explain how to opt out: %q", c.Long)
	}
}
