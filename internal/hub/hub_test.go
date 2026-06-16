package hub

import (
	"testing"
)

func newHub(t *testing.T) *Hub {
	t.Helper()
	h, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func TestNewAgentValidation(t *testing.T) {
	h := newHub(t)
	if err := h.NewAgent("Brokkr", "worker"); err == nil {
		t.Fatalf("uppercase name should be rejected")
	}
	if err := h.NewAgent("brokkr", "boss"); err == nil {
		t.Fatalf("bad role should be rejected")
	}
	if err := h.NewAgent("brokkr", "worker"); err != nil {
		t.Fatalf("valid agent: %v", err)
	}
	if err := h.NewAgent("brokkr", "worker"); err == nil {
		t.Fatalf("duplicate agent should be rejected")
	}
}

func TestNewAgentRecordsIdentityAndLog(t *testing.T) {
	h := newHub(t)
	if err := h.NewAgent("dvalin", "reviewer"); err != nil {
		t.Fatal(err)
	}
	st, err := h.State()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Agents) != 1 || st.Agents[0].Name != "dvalin" || st.Agents[0].Role != "reviewer" {
		t.Fatalf("unexpected state: %+v", st)
	}
	if st.Agents[0].Running { // podman absent → not running
		t.Fatalf("expected not running")
	}
	if st.Agents[0].Phase != "idle" {
		t.Fatalf("new agent should be idle: %+v", st.Agents[0])
	}
	// register event logged
	evs, _ := h.store.Events("dvalin", 0)
	if len(evs) != 1 || evs[0].Type != "register" {
		t.Fatalf("register not logged: %+v", evs)
	}
}

func TestTellUnknownAgent(t *testing.T) {
	h := newHub(t)
	if err := h.Tell("ghost", "hi", "user"); err == nil {
		t.Fatalf("telling unknown agent should error")
	}
}
