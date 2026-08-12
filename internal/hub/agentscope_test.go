package hub

import (
	"net/http/httptest"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAgentOfAnotherRepoIsFound is the Agents tab in global scope: it lists the whole fleet, so
// every row it offers an action on must be actionable. Resolving the name in the CALLER's repo made
// stop and delete answer "doesn't exist" about an agent the user was looking at.
func TestAgentOfAnotherRepoIsFound(t *testing.T) {
	h := newHub(t)
	if err := h.store.RegisterProject("ranke-ts", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := h.store.For("ranke-ts").PutAgent(store.Agent{Name: "fjalar", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	// A request from a different checkout — the header is the caller's repo, not fjalar's.
	req := httptest.NewRequest("POST", "/agent/stop", nil)
	req.Header.Set("X-Sindri-Project", t.TempDir())
	if got := h.agentReq(req, "fjalar"); got != "ranke-ts" {
		t.Errorf("agentReq = %q, want the agent's own project ranke-ts", got)
	}
}

// TestOwnRepoWinsANameClash: two repos may each have a "dvalin", and the one in the caller's own
// checkout is the one they mean. Same precedence a PR id gets.
func TestOwnRepoWinsANameClash(t *testing.T) {
	h := newHub(t)
	if err := h.store.RegisterProject("other", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := h.store.For("other").PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/agent/stop", nil)
	req.Header.Set("X-Sindri-Project", t.TempDir())
	own := h.reqProject(req) // registers the caller's repo under its own tag
	if err := h.store.For(own).PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if got := h.agentReq(req, "dvalin"); got != own {
		t.Errorf("agentReq = %q, want the caller's own project %q", got, own)
	}
}

// TestUnknownAgentStaysWithTheCaller: a name nobody has must not be silently rehomed — the caller's
// project is where the "no such agent" error belongs, so it names the repo they were working in.
func TestUnknownAgentStaysWithTheCaller(t *testing.T) {
	h := newHub(t)
	req := httptest.NewRequest("POST", "/agent/stop", nil)
	req.Header.Set("X-Sindri-Project", t.TempDir())
	if got := h.agentReq(req, "nobody"); got != h.reqProject(req) {
		t.Errorf("agentReq = %q, want the caller's own project", got)
	}
}
