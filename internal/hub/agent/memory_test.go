package agent

import (
	"testing"

	"github.com/flo-at/sindri/internal/container"
)

// gib is a binary gigabyte, the unit memory limits are written in.
const gib = int64(1) << 30

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

// TestHeadroomCountsAgentsRatherThanPercent is the point of the figure: the question asked of it is
// "does another agent fit", and the count answers it where a fraction leaves the reader dividing by
// a default they would have to look up. With no runtime wired the default is the fallback (2g), so
// 10 GiB free is five more agents.
func TestHeadroomCountsAgentsRatherThanPercent(t *testing.T) {
	m := Headroom(container.Capacity{UsedBytes: 6 * gib, TotalBytes: 16 * gib, Basis: container.BasisInUse})
	if !m.Known() {
		t.Fatal("a reading with a total is a known figure")
	}
	if m.FreeBytes() != 10*gib {
		t.Errorf("free = %d bytes, want %d", m.FreeBytes(), 10*gib)
	}
	if m.AgentBytes != 2*gib {
		t.Errorf("agent size = %d bytes, want the default %d", m.AgentBytes, 2*gib)
	}
	if m.Fits != 5 {
		t.Errorf("fits = %d agents, want 5", m.Fits)
	}
	if m.Basis != container.BasisInUse {
		t.Errorf("basis = %q, want it carried through from the backend", m.Basis)
	}
}

// TestUnmeasuredIsNotEmpty: a backend that could not answer leaves the figure unknown. Reporting
// zero used of zero total renders as a machine with nothing free, which is a claim nobody made.
func TestUnmeasuredIsNotEmpty(t *testing.T) {
	if m := Headroom(container.Capacity{}); m.Known() || m.Fits != 0 {
		t.Errorf("an unanswered reading must stay unknown, got %+v", m)
	}
}

// TestOvercommittedFitsNothing: a shared-kernel host can hand out more than it has, so used past
// total is a real reading. Free is none — never a negative that would wrap into room for agents.
func TestOvercommittedFitsNothing(t *testing.T) {
	m := Headroom(container.Capacity{UsedBytes: 20 * gib, TotalBytes: 16 * gib, Basis: container.BasisInUse})
	if m.FreeBytes() != 0 || m.Fits != 0 {
		t.Errorf("an overcommitted host has no room, got free=%d fits=%d", m.FreeBytes(), m.Fits)
	}
}

// TestParseMemoryReadsTheFormsTheRuntimesTake: the fit count divides by this, so a limit the hub
// accepts (-> ValidMemory) and cannot parse would silently count nothing.
func TestParseMemoryReadsTheFormsTheRuntimesTake(t *testing.T) {
	for in, want := range map[string]int64{
		"3g": 3 * gib, "2G": 2 * gib, "1gb": gib, "512m": 512 << 20, "512MB": 512 << 20,
		"1024k": 1 << 20, "2048": 2048, " 4g ": 4 * gib,
		"": 0, "lots": 0, "4x": 0,
	} {
		if got := parseMemory(in); got != want {
			t.Errorf("parseMemory(%q) = %d, want %d", in, got, want)
		}
	}
}
