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

// TestHeadroomCarriesTheReadingThrough: bytes and basis pass straight from the backend's reading —
// which agents are running against how many exist is the board's own question now (BoardState),
// not this figure's.
func TestHeadroomCarriesTheReadingThrough(t *testing.T) {
	m := Headroom(container.Capacity{UsedBytes: 6 * gib, TotalBytes: 16 * gib, Basis: container.BasisInUse})
	if !m.Known() {
		t.Fatal("a reading with a total is a known figure")
	}
	if m.FreeBytes() != 10*gib {
		t.Errorf("free = %d bytes, want %d", m.FreeBytes(), 10*gib)
	}
	if m.Basis != container.BasisInUse {
		t.Errorf("basis = %q, want it carried through from the backend", m.Basis)
	}
}

// TestUnmeasuredIsNotEmpty: a backend that could not answer leaves the figure unknown. Reporting
// zero used of zero total renders as a machine with nothing free, which is a claim nobody made.
func TestUnmeasuredIsNotEmpty(t *testing.T) {
	if m := Headroom(container.Capacity{}); m.Known() {
		t.Errorf("an unanswered reading must stay unknown, got %+v", m)
	}
}

// TestOvercommittedHasNoFreeBytes: a shared-kernel host can hand out more than it has, so used past
// total is a real reading. Free is none — never a negative.
func TestOvercommittedHasNoFreeBytes(t *testing.T) {
	m := Headroom(container.Capacity{UsedBytes: 20 * gib, TotalBytes: 16 * gib, Basis: container.BasisInUse})
	if m.FreeBytes() != 0 {
		t.Errorf("an overcommitted host has no free bytes, got %d", m.FreeBytes())
	}
}
