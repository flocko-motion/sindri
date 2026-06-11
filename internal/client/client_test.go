package client

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub"
)

// Exercise the full socket path end to end: hub server + HTTP client + store,
// for the podman-free operations (register + state).
func TestServeAndClientRoundTrip(t *testing.T) {
	root := t.TempDir()
	h, err := hub.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	go h.Serve()

	deadline := time.Now().Add(2 * time.Second)
	for !hub.IsRunning(root) {
		if time.Now().After(deadline) {
			t.Fatal("hub never came up")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cl := Dial(root)
	if err := cl.NewAgent("brokkr", "worker"); err != nil {
		t.Fatalf("client NewAgent: %v", err)
	}
	st, err := cl.State()
	if err != nil {
		t.Fatalf("client State: %v", err)
	}
	if len(st) != 1 || st[0].Name != "brokkr" {
		t.Fatalf("unexpected state over socket: %+v", st)
	}
	// The hub's domain error must surface across the socket.
	if err := cl.NewAgent("brokkr", "worker"); err == nil {
		t.Fatalf("expected duplicate error over socket")
	}
}
