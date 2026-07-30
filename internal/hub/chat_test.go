package hub

import (
	"strings"
	"testing"
)

// TestChatAddUnknownAgent: adding a non-existent agent is a clear error, not a
// silent phantom membership.
func TestChatAddUnknownAgent(t *testing.T) {
	h := newHub(t)
	if err := h.chat.Add(testProject, "ghost"); err == nil {
		t.Fatalf("adding an unknown agent should error")
	}
}

// TestChatAddRemoveAndGate: adding a real agent makes it a chatroom member (so the
// registry surfaces the `chat` verb via caller().InChat); removing clears it; a
// double add / double remove is reported as an error the operator can see.
func TestChatAddRemoveAndGate(t *testing.T) {
	h := newHub(t)
	if _, err := h.agents.NewAgent(testProject, "nori", "worker", ""); err != nil {
		t.Fatal(err)
	}

	c, err := h.caller(testProject, "nori")
	if err != nil {
		t.Fatal(err)
	}
	if c.InChat {
		t.Fatalf("agent should not be in chat before being added")
	}

	if err := h.chat.Add(testProject, "nori"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := h.chat.Add(testProject, "nori"); err == nil {
		t.Fatalf("re-adding an existing member should error")
	}
	if c, err = h.caller(testProject, "nori"); err != nil {
		t.Fatal(err)
	} else if !c.InChat {
		t.Fatalf("caller should be InChat after add (gates the chat verb)")
	}

	if err := h.chat.Remove(testProject, "nori"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := h.chat.Remove(testProject, "nori"); err == nil {
		t.Fatalf("removing a non-member should error")
	}
}

// TestChatSlashCommands: a user line starting with "/" is an in-chat command run
// by the hub (add/remove membership), while a plain line is broadcast as [user].
func TestChatSlashCommands(t *testing.T) {
	h := newHub(t)
	if _, err := h.agents.NewAgent(testProject, "nori", "worker", ""); err != nil {
		t.Fatal(err)
	}

	if err := h.chat.UserMessage("/add nori"); err != nil {
		t.Fatalf("/add: %v", err)
	}
	if m, _ := h.store.ChatIsMember(testProject, "nori"); !m {
		t.Fatalf("/add should make nori a member")
	}

	if err := h.chat.UserMessage("/remove nori"); err != nil {
		t.Fatalf("/remove: %v", err)
	}
	if m, _ := h.store.ChatIsMember(testProject, "nori"); m {
		t.Fatalf("/remove should drop nori's membership")
	}

	// An unknown command and a bad target are reported (system reply), not errors.
	if err := h.chat.UserMessage("/bogus"); err != nil {
		t.Fatalf("unknown command should not error: %v", err)
	}
	if err := h.chat.UserMessage("/add ghost"); err != nil {
		t.Fatalf("adding an unknown agent should not error (it replies): %v", err)
	}
	if m, _ := h.store.ChatIsMember(testProject, "ghost"); m {
		t.Fatalf("a typo'd /add must not create a phantom member")
	}

	// A plain line broadcasts as the user.
	if err := h.chat.UserMessage("hello team"); err != nil {
		t.Fatalf("plain message: %v", err)
	}
	log, _ := h.chat.Transcript(0)
	var sawUser bool
	for _, m := range log {
		if m.Sender == "user" && m.Body == "hello team" {
			sawUser = true
		}
	}
	if !sawUser {
		t.Fatalf("a non-slash line should be broadcast as [user], transcript=%+v", log)
	}
}

// TestChatSayRecordsTranscript: a user broadcast is recorded in the room transcript
// even with no member agents to inject into (the transcript is the durable record;
// delivery to agents is best-effort on top).
func TestChatSayRecordsTranscript(t *testing.T) {
	h := newHub(t)
	if _, err := h.chat.Say("why is the API flaky?"); err != nil {
		t.Fatalf("say: %v", err)
	}
	log, err := h.chat.Transcript(0)
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	if len(log) != 1 || log[0].Sender != "user" || !strings.Contains(log[0].Body, "flaky") {
		t.Fatalf("transcript = %+v, want one user message about flaky", log)
	}

	// An empty message is refused, not recorded.
	if _, err := h.chat.Say("   "); err == nil {
		t.Fatalf("empty message should be refused")
	}
	if log, _ = h.chat.Transcript(0); len(log) != 1 {
		t.Fatalf("empty message must not be recorded, transcript = %+v", log)
	}
}
