package agent

import "testing"

// TestConfiguredMemoryWinsAndUnsetNamesSomething: the per-agent value always wins, and an unset one
// resolves to a figure — with no runtime wired (a worker-only process, a test) the hub still names
// a fallback rather than leaving an empty cell where a limit should be.
func TestConfiguredMemoryWinsAndUnsetNamesSomething(t *testing.T) {
	if got := MemoryOrDefault("8g"); got != "8g" {
		t.Errorf("configured memory = %q, want it to win over any default", got)
	}
	if got := MemoryOrDefault("  4g  "); got != "4g" {
		t.Errorf("configured memory = %q, want it trimmed", got)
	}
	if got := MemoryOrDefault(""); got == "" {
		t.Error("an unset limit must still resolve to a figure")
	}
}
