package chat

import "testing"

// noDelivery is a Delivery that does nothing — presence logic needs no real
// injection or notification.
type noDelivery struct{}

func (noDelivery) Inject(project, name, text string) error { return nil }
func (noDelivery) Running(project, name string) bool       { return false }
func (noDelivery) Notify()                                 {}

// TestPresenceLock: the room is locked until the user heartbeats, and locks again
// once the last heartbeat ages past the TTL (the required-participant rule).
func TestPresenceLock(t *testing.T) {
	s := New(nil, noDelivery{}) // presence never touches the store
	if s.Present() {
		t.Fatal("no heartbeat yet — the room should be locked")
	}
	s.Heartbeat()
	if !s.Present() {
		t.Fatal("after a heartbeat the room should be unlocked")
	}
	s.seen = s.seen.Add(-2 * presenceTTL) // simulate the user going quiet
	if s.Present() {
		t.Fatal("a stale heartbeat should re-lock the room")
	}
}
