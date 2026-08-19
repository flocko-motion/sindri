package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// mailListFleet seeds one project with a worker, a reviewer and a planner — the three roles `mail
// list` treats differently (a coauthor is scoped identically to a planner, so it needs no case of
// its own here).
func mailListFleet(t *testing.T) *Hub {
	t.Helper()
	h := newHub(t)
	for _, a := range []store.Agent{
		{Name: "dvalin", Role: "worker", Workspace: "ws"},
		{Name: "thrain", Role: "reviewer", Workspace: "ws"},
		{Name: "galar", Role: "planner", Workspace: "ws"},
	} {
		if err := h.store.For(testProject).PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

// TestMailListScopesAWorkerToSenderOrRecipient: dvalin sees what it sent or was sent, and nothing
// addressed between two other agents in the same project.
func TestMailListScopesAWorkerToSenderOrRecipient(t *testing.T) {
	h := mailListFleet(t)
	ps := h.store.For(testProject)
	self := h.repoName(testProject) + "/dvalin"
	toDvalin, err := ps.AddMail("dvalin", "hub", "to dvalin from hub", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	fromDvalin, err := ps.AddMail("galar", self, "to galar from dvalin", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	notDvalins, err := ps.AddMail("galar", "hub", "to galar from hub, none of dvalin's business", false, 0)
	if err != nil {
		t.Fatal(err)
	}

	out, code := execAs(t, h, "dvalin", "mail", "list")
	if code != 0 {
		t.Fatalf("mail list failed (%d): %s", code, out)
	}
	if !strings.Contains(out, api.MailID(toDvalin.ID)) {
		t.Errorf("dvalin should see mail addressed to it: %s", out)
	}
	if !strings.Contains(out, api.MailID(fromDvalin.ID)) {
		t.Errorf("dvalin should see mail it sent: %s", out)
	}
	if strings.Contains(out, api.MailID(notDvalins.ID)) {
		t.Errorf("dvalin must not see mail between two other agents: %s", out)
	}

	// A reviewer party to none of these three sees nothing.
	out, code = execAs(t, h, "thrain", "mail", "list")
	if code != 0 {
		t.Fatalf("mail list failed (%d): %s", code, out)
	}
	for _, id := range []int64{toDvalin.ID, fromDvalin.ID, notDvalins.ID} {
		if strings.Contains(out, api.MailID(id)) {
			t.Errorf("thrain is party to none of this project's mail: %s", out)
		}
	}
}

// TestMailListGivesAPlannerTheWholeProject: repo-scoped, mirroring how planner task listing is
// already repo-scoped (sd-3467ab) — a planner sees every message, whoever it was to or from.
func TestMailListGivesAPlannerTheWholeProject(t *testing.T) {
	h := mailListFleet(t)
	ps := h.store.For(testProject)
	self := h.repoName(testProject) + "/dvalin"
	toDvalin, err := ps.AddMail("dvalin", "hub", "to dvalin from hub", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	betweenOthers, err := ps.AddMail("thrain", self, "to thrain from dvalin", false, 0)
	if err != nil {
		t.Fatal(err)
	}

	out, code := execAs(t, h, "galar", "mail", "list")
	if code != 0 {
		t.Fatalf("mail list failed (%d): %s", code, out)
	}
	for _, id := range []int64{toDvalin.ID, betweenOthers.ID} {
		if !strings.Contains(out, api.MailID(id)) {
			t.Errorf("a planner should see every message in its project: %s", out)
		}
	}
}

// TestMailListWordIsReservedAheadOfSend: "list" must resolve to the listing verb, never to a
// (nonexistent) recipient literally named "list" — the collision this reservation exists to close.
func TestMailListWordIsReservedAheadOfSend(t *testing.T) {
	h := mailListFleet(t)
	out, code := execAs(t, h, "dvalin", "mail", "list")
	if code != 0 {
		t.Fatalf("mail list failed (%d): %s", code, out)
	}
	if strings.Contains(out, "no agent named") {
		t.Errorf("list must never be read as a recipient name: %s", out)
	}
}

// TestMailShowStaysARecipientTypo: unlike "list", "show" is NOT reserved under mail — `mail show
// ml-465` keeps meaning "send agent show the text ml-465", exactly as it did before this subtask,
// and still fails as an unknown recipient rather than being swallowed into `show`'s own grammar.
func TestMailShowStaysARecipientTypo(t *testing.T) {
	h := mailListFleet(t)
	out, code := execAs(t, h, "dvalin", "mail", "show", "ml-465")
	if code == 0 || !strings.Contains(out, "no agent named") {
		t.Errorf("expected a refusal naming the problem (%d): %s", code, out)
	}
}
