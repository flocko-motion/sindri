package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// view builds a transcript snapshot for the renderer. lipgloss strips styling when stdout
// isn't a terminal, so these assertions see the plain layout — exactly the structure under
// test (colour is ui/theme's business, covered there).
func view(msgs ...store.ChatMessage) hub.ChatView {
	return hub.ChatView{
		Members: []store.ChatMember{{Name: "eitri", Role: "worker"}},
		Log:     msgs,
	}
}

func msg(id int64, sender, body string) store.ChatMessage {
	return store.ChatMessage{ID: id, Sender: sender, Body: body, TS: "2026-07-28T09:00:00Z"}
}

// TestTranscriptSeparatesSpeakers: a change of speaker gets a blank line and a fresh
// header, which is what makes a long transcript skimmable instead of a wall of text.
func TestTranscriptSeparatesSpeakers(t *testing.T) {
	got := renderChat(view(
		msg(1, "user", "what's blocking?"),
		msg(2, "eitri", "the rebase"),
	))
	lines := strings.Split(got, "\n")
	var headers []int
	for i, l := range lines {
		if strings.Contains(l, "user") && strings.Contains(l, "👤") {
			headers = append(headers, i)
		}
		if strings.Contains(l, "eitri") && strings.Contains(l, "🤖") {
			headers = append(headers, i)
		}
	}
	if len(headers) < 2 {
		t.Fatalf("expected a header per speaker, got:\n%s", got)
	}
	// The line directly above the second speaker's header must be blank.
	if second := headers[len(headers)-1]; strings.TrimSpace(lines[second-1]) != "" {
		t.Errorf("a new speaker needs a blank line above it, got:\n%s", got)
	}
}

// TestTranscriptGroupsSameSpeaker: consecutive messages from one participant repeat
// neither the blank line nor the header — that's the grouping that keeps a back-and-forth
// compact.
func TestTranscriptGroupsSameSpeaker(t *testing.T) {
	got := renderChat(view(
		msg(1, "eitri", "first"),
		msg(2, "eitri", "second"),
	))
	if n := strings.Count(got, "eitri"); n != 2 { // once in the roster, once as the header
		t.Errorf("a run from one speaker should print one header, got %d mentions:\n%s", n, got)
	}
	if !strings.Contains(got, "  first") || !strings.Contains(got, "  second") {
		t.Errorf("both bodies should be indented under the one header, got:\n%s", got)
	}
}

// TestMultilineBodyKeepsIndent: every line of a multi-line message stays under the header,
// so a code block pasted into the room doesn't break the attribution.
func TestMultilineBodyKeepsIndent(t *testing.T) {
	got := renderChat(view(msg(1, "eitri", "line one\nline two")))
	for _, want := range []string{"  line one", "  line two"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing indented %q in:\n%s", want, got)
		}
	}
}

// TestRosterAlwaysIncludesUser: the stored roster holds only agents, so an empty one used
// to render "the meeting room is empty" at a user who was standing in it. The user is a
// permanent participant and must always be listed — and an empty roster must say what is
// actually missing (agents) plus how to fix it.
func TestRosterAlwaysIncludesUser(t *testing.T) {
	empty := renderMembers(hub.ChatView{})
	if !strings.Contains(empty, "user") || !strings.Contains(empty, hub.ChatUserIcon) {
		t.Errorf("the user must be listed even with no agents, got: %q", empty)
	}
	if strings.Contains(empty, "is empty") {
		t.Errorf("must not claim the room is empty while the user is in it, got: %q", empty)
	}
	if !strings.Contains(empty, "no agents yet") || !strings.Contains(empty, "meeting add") {
		t.Errorf("an agentless room should name what's missing and how to fix it, got: %q", empty)
	}

	// With agents: the user first, then each agent with its role.
	full := renderMembers(hub.ChatView{Members: []store.ChatMember{
		{Name: "eitri", Role: "worker"}, {Name: "dvalin", Role: "reviewer"},
	}})
	if strings.Index(full, "user") > strings.Index(full, "eitri") {
		t.Errorf("the user should lead the roster, got: %q", full)
	}
	for _, want := range []string{"eitri", "worker", "dvalin", "reviewer"} {
		if !strings.Contains(full, want) {
			t.Errorf("roster missing %q, got: %q", want, full)
		}
	}
	if strings.Contains(full, "no agents yet") {
		t.Errorf("a populated roster should not nag, got: %q", full)
	}
}

// TestHumanIsMarked is the property agents depend on and the user asked for: the human is
// visibly not another agent.
func TestHumanIsMarked(t *testing.T) {
	got := renderChat(view(msg(1, "user", "hello"), msg(2, "eitri", "hi")))
	if !strings.Contains(got, hub.ChatUserIcon+" user") {
		t.Errorf("the user needs the human icon, got:\n%s", got)
	}
	if !strings.Contains(got, hub.ChatAgentIcon+" eitri") {
		t.Errorf("an agent needs the robot icon, got:\n%s", got)
	}
}
