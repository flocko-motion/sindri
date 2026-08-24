package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
)

// TestCmdShowMailReadsAnyMessageWithoutMarkingIt is the deliberate width: a mail id is unguessable,
// so the only way an agent not addressed by a message ever learns its id is a human handing it over
// directly — and reading it back must not consume the true recipient's own unread mark.
func TestCmdShowMailReadsAnyMessageWithoutMarkingIt(t *testing.T) {
	e, ps := runEngine(t)
	m, err := ps.AddMail("dvalin", "hub", "a message for someone else entirely", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := registry.Caller{Project: "repo", Agent: "bombur", Role: "worker"} // not the recipient
	var out bytes.Buffer
	if code, err := e.CmdShow(c, []string{api.MailID(m.ID)}, &out); err != nil || code != 0 {
		t.Fatalf("CmdShow(%s): code=%d err=%v", api.MailID(m.ID), code, err)
	}
	if !strings.Contains(out.String(), "a message for someone else entirely") {
		t.Errorf("should print the body: %q", out.String())
	}
	stored, ok, err := e.store.MailByID(m.ID)
	if err != nil || !ok {
		t.Fatalf("MailByID(%d): ok=%v err=%v", m.ID, ok, err)
	}
	if stored.Read() {
		t.Error("show ml-<id> must not mark the message read — only the true recipient's own path may")
	}
}

// TestCmdShowMailUnknownID: a well-formed id nobody used is a clean refusal, not a crash.
func TestCmdShowMailUnknownID(t *testing.T) {
	e, _ := runEngine(t)
	c := registry.Caller{Project: "repo", Agent: "bombur", Role: "worker"}
	var out bytes.Buffer
	code, err := e.CmdShow(c, []string{"ml-999999"}, &out)
	if err != nil || code != 1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "no such message") {
		t.Errorf("should say so plainly: %q", out.String())
	}
}

// TestCmdShowRejectsUnrecognisedIDShapes covers the crash this subtask exists to fix: an id from no
// family show recognises must be a usage error, never a lookup that fails and comes back masked as
// an internal error.
func TestCmdShowRejectsUnrecognisedIDShapes(t *testing.T) {
	e, _ := runEngine(t)
	c := registry.Caller{Project: "repo", Agent: "bombur", Role: "worker"}
	for _, id := range []string{"ml-465", "sd-1234", "wibble"} {
		var out bytes.Buffer
		code, err := e.CmdShow(c, []string{id}, &out)
		if id == "ml-465" {
			// A well-shaped mail id that doesn't exist is CmdShowMail's own clean refusal.
			if err != nil || code != 1 || !strings.Contains(out.String(), "no such message") {
				t.Errorf("%s: code=%d err=%v out=%q", id, code, err, out.String())
			}
			continue
		}
		if err != nil || code != 2 {
			t.Errorf("%s: code=%d err=%v, want a usage error", id, code, err)
		}
	}
}
