package tui

import (
	"errors"
	"strings"
	"testing"
)

// composerWith opens the Meeting composer holding draft.
func composerWith(t *testing.T, draft string) model {
	t.Helper()
	m := newModel(nil, nil, "")
	m.tab = 4 // Meeting
	m.w, m.h = 100, 40
	m.startComposing()
	m.composer.SetValue(draft)
	return m
}

// send drives one keypress through the composer and returns the resulting model.
func send(t *testing.T, m model, key string) model {
	t.Helper()
	next, _ := m.updateComposer(keyMsg(key))
	got, ok := next.(model)
	if !ok {
		t.Fatalf("updateComposer returned %T", next)
	}
	return got
}

// TestComposerClosesOnSend is the reported bug: the composer used to stay open until the hub had
// finished typing the line into every agent's session, so the keyboard felt stuck for seconds.
// It must close on the keystroke, before any of that work.
func TestComposerClosesOnSend(t *testing.T) {
	for _, key := range []string{"ctrl+s", "enter"} {
		draft := "hello room"
		if key == "enter" {
			draft = "/who" // enter submits a command; a message would take a newline
		}
		m := send(t, composerWith(t, draft), key)
		if m.composing {
			t.Errorf("%s: the composer must close at once, not after the send completes", key)
		}
	}
}

// TestComposerEnterSubmitsOnlyCommands: "/add nori" then Enter should act, which is what the report
// asked for — while a message keeps Enter as a newline, since messages are multiline.
func TestComposerEnterSubmitsOnlyCommands(t *testing.T) {
	cmd := send(t, composerWith(t, "/add nori"), "enter")
	if cmd.composing {
		t.Error("a command must submit on enter")
	}

	msg := composerWith(t, "first line")
	after := send(t, msg, "enter")
	if !after.composing {
		t.Fatal("a message must stay open on enter — enter is a newline for multiline text")
	}
	if got := after.composer.Value(); !strings.Contains(got, "\n") {
		t.Errorf("enter should have inserted a newline, value = %q", got)
	}
}

// TestComposerSendIsAcknowledged: "accepted" has to be visible, or closing the field is the only
// feedback and a slow hub looks like a dropped message.
func TestComposerSendIsAcknowledged(t *testing.T) {
	m := send(t, composerWith(t, "hello room"), "ctrl+s")
	if !strings.Contains(m.flash, "sending") {
		t.Errorf("the send should be acknowledged immediately, flash = %q", m.flash)
	}
}

// TestComposerKeepsDraftUntilAccepted: the draft survives the close, so a rejected message (over
// the hub's length cap) can be trimmed instead of retyped. It is cleared once the hub has it.
func TestComposerKeepsDraftUntilAccepted(t *testing.T) {
	m := send(t, composerWith(t, "a message the hub might refuse"), "ctrl+s")
	if m.composer.Value() == "" {
		t.Fatal("the draft must be kept until the hub accepts it")
	}

	back, _ := m.Update(chatFailedMsg{err: errors.New("too long"), draft: "a message the hub might refuse"})
	failed := back.(model)
	if !failed.composing {
		t.Error("a refused send should reopen the composer to fix the text")
	}
	if !strings.Contains(failed.composer.Value(), "the hub might refuse") {
		t.Errorf("the refused draft must come back, got %q", failed.composer.Value())
	}
	if failed.errText == "" {
		t.Error("the reason for the refusal must be shown")
	}

	ok, _ := m.Update(chatSentMsg{})
	if got := ok.(model).composer.Value(); got != "" {
		t.Errorf("an accepted send clears the draft, got %q", got)
	}
}

// TestComposerEscKeepsTheDraftClosed: esc is "not now" — it must not send.
func TestComposerEscKeepsTheDraftClosed(t *testing.T) {
	m := send(t, composerWith(t, "/who"), "esc")
	if m.composing {
		t.Error("esc must close the composer")
	}
	if strings.Contains(m.flash, "sending") {
		t.Errorf("esc must not send, flash = %q", m.flash)
	}
}
