package applecontainer

import "testing"

// TestReservedIsWhatTheFleetHolds: each pod is a micro-VM that takes its whole limit from the Mac
// when it starts, so the sum of the limits is what the fleet holds and what decides whether the
// next agent starts — one whose reservation does not fit is refused however quiet the fleet is.
func TestReservedIsWhatTheFleetHolds(t *testing.T) {
	const raw = `[
	  {"id":"a","status":{"state":"running"},"configuration":{"id":"a","resources":{"cpus":4,"memoryInBytes":2147483648}}},
	  {"id":"b","status":{"state":"running"},"configuration":{"id":"b","resources":{"cpus":4,"memoryInBytes":4294967296}}}
	]`
	entries, err := parseInspect("ls --format json", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := reservedBytes(entries), int64(2147483648+4294967296); got != want {
		t.Errorf("reserved = %d, want the sum of the limits (%d)", got, want)
	}
	if got := reservedBytes(nil); got != 0 {
		t.Errorf("an empty fleet reserves nothing, got %d", got)
	}
}
