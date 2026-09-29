package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// typedPane is a real input box with somebody's unsent line in it. The chevron matters: it is what
// the classifier reads the box by, and the idle fixtures elsewhere here draw a plain ">" prompt.
const typedPane = "╭──────────────────────────────────────╮\n" +
	"❯ ranke-db is not an issue now, we focus on the webapp\n" +
	"╰──────────────────────────────────────╯"

// emptyBoxPane is the same box with nothing in it — a pane that will take what it is told.
const emptyBoxPane = "╭──────────────────────────────────────╮\n❯ \n╰──────────────────────────────────────╯"

// TestAPushWaitsForSomebodyToFinishTyping is the reported bug. send-keys APPENDS to whatever is in
// the input box and then sends Enter, so a notice pushed at a pane somebody is typing in joins their
// half-written sentence and submits the pair. The user watched their own message go up with a hub
// mail notice spliced into the middle of it.
//
// Every agent's pane is potentially somebody's seat — a coauthor shares one by design and any other
// can be attached to — so this is not a rule about roles. And the push is HELD, not dropped: the
// message is wanted, it just must not land mid-sentence, so it goes the moment the line clears.
func TestAPushWaitsForSomebodyToFinishTyping(t *testing.T) {
	s, f := tellFixture(t, "eitri", emptyBoxPane)
	f.typedFor, f.typedBox = 2, typedPane // two looks find them typing, the third finds them done

	if err := s.Inject(t.Context(), "proj", "eitri", "[hub] You have 1 unread message(s) waiting"); err != nil {
		t.Fatalf("a push should wait for the line to clear and then go: %v", err)
	}
	if f.typedFor != 0 {
		t.Errorf("it did not wait out the typing — %d looks left unspent", f.typedFor)
	}
	if len(f.sent) != 1 || !strings.Contains(f.sent[0], "unread message") {
		t.Errorf("the message should have reached the session once the box was free, got %q", f.sent)
	}
}

// TestALineLeftBehindDoesNotDeafenAnAgent is the other end of the wait. Somebody who walks away
// mid-line would otherwise hold every push to that agent for ever, and an agent nothing can reach
// is worse than a notice arriving beside an abandoned sentence.
func TestALineLeftBehindDoesNotDeafenAnAgent(t *testing.T) {
	s, f := tellFixture(t, "bifur", typedPane)
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the bound is a minute; a caller that gives up first is the same exit, reached at once

	err := s.Inject(ctx, "proj", "bifur", "[hub] the gate passed")
	if err == nil {
		t.Fatal("a push that never found a free box must report, not report success")
	}
	if len(f.sent) != 0 {
		t.Errorf("nothing should have been typed into somebody's line, got %q", f.sent)
	}
}

// TestAnEmptyInputBoxTakesThePush is the other half: the guard must not silence the fleet. An empty
// box is the ordinary case and it still takes what it is told.
func TestAnEmptyInputBoxTakesThePush(t *testing.T) {
	s, f := tellFixture(t, "dvalin", emptyBoxPane)
	if err := s.Inject(t.Context(), "proj", "dvalin", "carry on"); err != nil {
		t.Fatalf("an empty box must still take a push: %v", err)
	}
	if len(f.sent) != 1 || !strings.Contains(f.sent[0], "carry on") {
		t.Errorf("the message should have reached the session, got %q", f.sent)
	}
}

// TestSendAnywayOverrulesSomebodyTyping: the same escape the signed-out refusal has. A user who
// knows the line in that box is their own abandoned one can say so.
func TestSendAnywayOverrulesSomebodyTyping(t *testing.T) {
	s, f := tellFixture(t, "nori", typedPane)
	if err := s.Tell(t.Context(), "proj", "nori", "carry on", "user", api.SignedOutSend); err != nil {
		t.Fatalf("send-anyway must deliver: %v", err)
	}
	if len(f.sent) != 1 {
		t.Errorf("the message should have been delivered once, got %q", f.sent)
	}
}
