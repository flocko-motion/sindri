package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestUnretiringTellsTheAgent is sd-9d36d2: DirRetired sends an agent away from ever asking again,
// so the push on the false transition of SetRetired is the only channel that would reach it.
func TestUnretiringTellsTheAgent(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Retired: true}); err != nil {
		t.Fatal(err)
	}
	if err := h.SetRetired(testProject, "dvalin", false); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("un-retiring should have mailed the agent, got %+v (err %v)", unread, err)
	}
	if !strings.Contains(unread[0].Body, "back in service") {
		t.Errorf("the message should say it is back in service: %q", unread[0].Body)
	}
}

// TestRetiringSaysNothingToTheAgent: winding an agent down is not itself news to push — DirRetired
// is served on its next ordinary ask, same as any other resting directive.
func TestRetiringSaysNothingToTheAgent(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := h.SetRetired(testProject, "dvalin", true); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	if all, _ := h.store.AllMail(0); len(all) != 0 {
		t.Errorf("retiring should not mail the agent, got %+v", all)
	}
}

// TestUnretiringAnAlreadyActiveAgentIsANoOp: `--back` on an agent that was never retired is not a
// real transition, so it must not manufacture a wake nobody needs.
func TestUnretiringAnAlreadyActiveAgentIsANoOp(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := h.SetRetired(testProject, "dvalin", false); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	if all, _ := h.store.AllMail(0); len(all) != 0 {
		t.Errorf("un-retiring one that was never retired should be quiet, got %+v", all)
	}
}
