package pod

import "testing"

// TestPodmanDefaultMemory: -m on a shared-kernel container is a ceiling the process grows into, so
// headroom is free until used — unlike a micro-VM, where the limit is reserved from the host whether
// used or not. The two backends therefore answer differently, which is why the port asks rather than
// the hub deciding: 1GiB (the runtime's own) OOM-kills a worker building Go under Claude Code.
func TestPodmanDefaultMemory(t *testing.T) {
	if got := (Engine{}).DefaultMemory(); got != "3g" {
		t.Errorf("DefaultMemory = %q, want 3g", got)
	}
}
