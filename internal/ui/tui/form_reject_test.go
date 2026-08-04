package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// formWith opens a two-field form whose apply returns whatever outcome a test wants, so the
// submit round trip can be driven without a hub.
func formWith(t *testing.T, outcome tea.Msg) model {
	t.Helper()
	m := newModel(nil, nil, "")
	m.w, m.h = 100, 40
	title := newTextField("title", "short")
	desc := newTextareaField("description", "a long description that took real effort to write")
	m.form.open("new task", []field{title, desc}, nil, func() tea.Cmd {
		return func() tea.Msg { return outcome }
	})
	return m
}

// submit presses ctrl+s and feeds the resulting message back, returning the settled model.
func submit(t *testing.T, m model) model {
	t.Helper()
	cmd := m.form.update(keyMsg("ctrl+s"))
	if cmd == nil {
		t.Fatal("ctrl+s produced no submit")
	}
	next, _ := m.Update(cmd())
	got, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return got
}

// TestRejectedFormKeepsEverythingTyped is the reported bug: a title td refuses (too short) closed
// the modal and took the description with it. The form must stay open, still filled, with the
// reason visible.
func TestRejectedFormKeepsEverythingTyped(t *testing.T) {
	m := formWith(t, errModalMsg{errors.New("title too short (5 chars, need 15)")})
	after := submit(t, m)

	if !after.form.active {
		t.Fatal("a refused submit must leave the form open")
	}
	if after.form.submitting {
		t.Error("the in-flight marker should clear once the answer arrives")
	}
	if !strings.Contains(after.form.err, "too short") {
		t.Errorf("the form should show why it was refused, got %q", after.form.err)
	}
	if got := after.form.fields[1].value(); !strings.Contains(got, "real effort") {
		t.Errorf("the description was lost: %q", got)
	}
	if got := after.form.fields[0].value(); got != "short" {
		t.Errorf("the title should still be there to fix, got %q", got)
	}
	// And the error belongs to the form, not the full-screen modal that hides it.
	if after.errText != "" {
		t.Errorf("a form refusal should not open the error modal, got %q", after.errText)
	}
}

// TestAcceptedFormClosesAndForwards: the form must still close on success, and the apply's own
// message must reach Update — otherwise the board would not show what the form just changed.
func TestAcceptedFormClosesAndForwards(t *testing.T) {
	m := formWith(t, resumedMsg{})
	cmd := m.form.update(keyMsg("ctrl+s"))
	msg := cmd()

	applied, ok := msg.(formAppliedMsg)
	if !ok {
		t.Fatalf("expected formAppliedMsg, got %T", msg)
	}
	if _, ok := applied.inner.(resumedMsg); !ok {
		t.Errorf("the apply's own message must be carried, got %T", applied.inner)
	}
	next, forwarded := m.Update(msg)
	after := next.(model)
	if after.form.active || after.form.submitting {
		t.Error("an accepted submit must close the form")
	}
	if forwarded == nil {
		t.Fatal("the inner message must be forwarded so the board updates")
	}
	if _, ok := forwarded().(resumedMsg); !ok {
		t.Error("the forwarded message should be the apply's own")
	}
}

// TestFormRefusesASecondSubmitInFlight: ctrl+s twice on a slow hub would create two tasks.
func TestFormRefusesASecondSubmitInFlight(t *testing.T) {
	m := formWith(t, resumedMsg{})
	if cmd := m.form.update(keyMsg("ctrl+s")); cmd == nil {
		t.Fatal("the first submit should fire")
	}
	if cmd := m.form.update(keyMsg("ctrl+s")); cmd != nil {
		t.Error("a second submit while one is in flight must be ignored")
	}
}

// TestClientValidationStillStopsBeforeTheHub: a rule the front-end can check needs no round trip,
// and must not mark the form as submitting.
func TestClientValidationStillStopsBeforeTheHub(t *testing.T) {
	m := newModel(nil, nil, "")
	m.w, m.h = 100, 40
	f := newTextField("parent", "td-nope")
	m.form.open("new task", []field{f}, func() string { return "unknown parent: td-nope" }, func() tea.Cmd {
		t.Error("apply must not run when client validation fails")
		return nil
	})
	if cmd := m.form.update(keyMsg("ctrl+s")); cmd != nil {
		t.Error("a failed validation should not submit")
	}
	if !m.form.active || m.form.submitting {
		t.Error("the form stays open and is not in flight")
	}
	if !strings.Contains(m.form.err, "unknown parent") {
		t.Errorf("the validation message should show, got %q", m.form.err)
	}
}

// TestEscAbandonsAStuckSubmit: esc is the way out if the hub never answers.
func TestEscAbandonsAStuckSubmit(t *testing.T) {
	m := formWith(t, resumedMsg{})
	m.form.update(keyMsg("ctrl+s"))
	m.form.update(keyMsg("esc"))
	if m.form.active {
		t.Error("esc must close a form waiting on the hub")
	}
}
