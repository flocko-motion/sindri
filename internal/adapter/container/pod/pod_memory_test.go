package pod

import (
	"testing"

	"github.com/flo-at/sindri/internal/container"
)

// TestPodmanDefaultMemory: -m on a shared-kernel container is a ceiling the process grows into, so
// headroom is free until used — unlike a micro-VM, where the limit is reserved from the host whether
// used or not. The two backends therefore answer differently, which is why the port asks rather than
// the hub deciding: 1GiB (the runtime's own) OOM-kills a worker building Go under Claude Code.
func TestPodmanDefaultMemory(t *testing.T) {
	if got := (Engine{}).DefaultMemory(); got != "3g" {
		t.Errorf("DefaultMemory = %q, want 3g", got)
	}
}

// TestCapacityIsPodmansOwnAccountOfItsHost: on macOS podman runs containers inside its VM, so
// `podman info` describes that VM. Reading the machine's memory any other way would name a ceiling
// no container can reach — hence the reading comes from podman, and the parse must take its shape.
func TestCapacityIsPodmansOwnAccountOfItsHost(t *testing.T) {
	const raw = `{"host":{"arch":"arm64","memFree":6442450944,"memTotal":17179869184},"version":{}}`
	got, err := parseInfoMemory([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalBytes != 17179869184 {
		t.Errorf("total = %d, want podman's memTotal", got.TotalBytes)
	}
	if want := int64(17179869184 - 6442450944); got.UsedBytes != want {
		t.Errorf("used = %d, want total minus free (%d)", got.UsedBytes, want)
	}
	if got.Basis != container.BasisInUse {
		t.Errorf("basis = %q: a shared-kernel container takes memory as it uses it", got.Basis)
	}
}

// TestNoHostMemoryIsAnErrorNotAnEmptyMachine: an info payload without the figures (a shape change,
// an older podman) must fail loudly. Returned as a zero capacity it would render as a full machine
// with room for nothing.
func TestNoHostMemoryIsAnErrorNotAnEmptyMachine(t *testing.T) {
	for _, raw := range []string{`{"host":{}}`, `{}`, `not json`} {
		if _, err := parseInfoMemory([]byte(raw)); err == nil {
			t.Errorf("parseInfoMemory(%q) returned no error", raw)
		}
	}
}
