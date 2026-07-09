package hub

import (
	"strings"
	"testing"
)

// TestChatAddUnknownAgent: adding a non-existent agent is a clear error, not a
// silent phantom membership.
func TestChatAddUnknownAgent(t *testing.T) {
	h := newHub(t)
	if err := h.ChatAdd(testProject, "ghost"); err == nil {
		t.Fatalf("adding an unknown agent should error")
	}
}

// TestChatAddRemoveAndGate: adding a real agent makes it a chatroom member (so the
// registry surfaces the `chat` verb via caller().InChat); removing clears it; a
// double add / double remove is reported as an error the operator can see.
func TestChatAddRemoveAndGate(t *testing.T) {
	h := newHub(t)
	if _, err := h.NewAgent(testProject, "nori", "worker", ""); err != nil {
		t.Fatal(err)
	}

	c, err := h.caller(testProject, "nori")
	if err != nil {
		t.Fatal(err)
	}
	if c.InChat {
		t.Fatalf("agent should not be in chat before being added")
	}

	if err := h.ChatAdd(testProject, "nori"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := h.ChatAdd(testProject, "nori"); err == nil {
		t.Fatalf("re-adding an existing member should error")
	}
	if c, err = h.caller(testProject, "nori"); err != nil {
		t.Fatal(err)
	} else if !c.InChat {
		t.Fatalf("caller should be InChat after add (gates the chat verb)")
	}

	if err := h.ChatRemove(testProject, "nori"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := h.ChatRemove(testProject, "nori"); err == nil {
		t.Fatalf("removing a non-member should error")
	}
}

// TestChatSayRecordsTranscript: a user broadcast is recorded in the room transcript
// even with no member agents to inject into (the transcript is the durable record;
// delivery to agents is best-effort on top).
func TestChatSayRecordsTranscript(t *testing.T) {
	h := newHub(t)
	if _, err := h.ChatSay("why is the API flaky?"); err != nil {
		t.Fatalf("say: %v", err)
	}
	log, err := h.ChatTranscript(0)
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	if len(log) != 1 || log[0].Sender != "user" || !strings.Contains(log[0].Body, "flaky") {
		t.Fatalf("transcript = %+v, want one user message about flaky", log)
	}

	// An empty message is refused, not recorded.
	if _, err := h.ChatSay("   "); err == nil {
		t.Fatalf("empty message should be refused")
	}
	if log, _ = h.ChatTranscript(0); len(log) != 1 {
		t.Fatalf("empty message must not be recorded, transcript = %+v", log)
	}
}
