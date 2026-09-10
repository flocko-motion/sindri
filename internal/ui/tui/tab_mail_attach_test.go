package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// mailAttachModel is the Mail tab with one message selected and a roster of two real agents —
// callers set msg.Agent/Sender per case.
func mailAttachModel(agent, sender string) model {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.mailFilter = 6, api.MailAll
	m.state = api.BoardState{
		Agents: []api.AgentView{
			{Name: "nori", Project: "repo"},
			{Name: "dain", Project: "repo"},
		},
		Mail: []api.Mail{{ID: 1, Project: "repo", Repo: "one", Agent: agent, Sender: sender, Body: "x"}},
	}
	m.reclamp()
	return m
}

// TestMailAttachTargetsTheLiveAgentOnEitherSide is sd-fffd47's table: the three reserved,
// non-agent parties (hub, user, reviewer) in both the sender and the recipient position, plus the
// two directions that must keep working — a test covering only agent-to-agent mail passes today
// even with the bug in place.
func TestMailAttachTargetsTheLiveAgentOnEitherSide(t *testing.T) {
	cases := []struct {
		name          string
		agent, sender string
		wantAgent     string // "" = neither party is live
	}{
		{"agent to agent: recipient is the ordinary target", "nori", "dain", "nori"},
		{"agent to user: the sender is the only live party", api.SenderUser, "dain", "dain"},
		{"user to agent: the recipient is the only live party", "nori", api.SenderUser, "nori"},
		{"hub notice to the user: neither party is live", api.SenderUser, "hub", ""},
		{"reviewer rejection to the user: neither party is live", api.SenderUser, "reviewer", ""},
		{"hub notice to an unregistered name: neither party is live", "ghost", "hub", ""},
	}
	for _, c := range cases {
		m := mailAttachModel(c.agent, c.sender)
		a, ok := m.mailAttachTarget()
		if c.wantAgent == "" {
			if ok {
				t.Errorf("%s: expected no live target, got %q", c.name, a.Name)
			}
			continue
		}
		if !ok || a.Name != c.wantAgent {
			t.Errorf("%s: mailAttachTarget = %q (ok=%v), want %q", c.name, a.Name, ok, c.wantAgent)
		}
	}
}

// TestMailAttachTargetStripsACrossRepoSender: mailverb.go qualifies a sender as "repo/agent" so a
// reply knows where to answer, but the roster only carries the bare name.
func TestMailAttachTargetStripsACrossRepoSender(t *testing.T) {
	m := mailAttachModel(api.SenderUser, "elsewhere/dain")
	a, ok := m.mailAttachTarget()
	if !ok || a.Name != "dain" {
		t.Errorf("mailAttachTarget = %q (ok=%v), want dain", a.Name, ok)
	}
}

// TestMailAttachIsHiddenWhenNeitherPartyIsLive is sd-6ec1b9's rule applied here: an action that
// cannot succeed must not be offered, so pressing it is never how the user finds that out.
func TestMailAttachIsHiddenWhenNeitherPartyIsLive(t *testing.T) {
	hidden := mailAttachModel(api.SenderUser, "hub")
	if mailAttachable(hidden) {
		t.Error("mailAttachable should be false when neither party is a live agent")
	}
	if strings.Contains(hidden.footerFor(scopeMail), "attach") {
		t.Errorf("the footer must not advertise attach here: %q", hidden.footerFor(scopeMail))
	}

	live := mailAttachModel(api.SenderUser, "dain")
	if !mailAttachable(live) {
		t.Error("mailAttachable should be true when the sender is a live agent")
	}
	if !strings.Contains(live.footerFor(scopeMail), "attach") {
		t.Errorf("the footer should advertise attach here: %q", live.footerFor(scopeMail))
	}
}

// TestMailAttachNoLongerLeaksTheOldRosterError: pressing attach directly (bypassing the footer)
// on a hub notice to the user must not name the reserved word "user" as a missing roster agent —
// the whole defect class this task exists to close.
func TestMailAttachNoLongerLeaksTheOldRosterError(t *testing.T) {
	m := mailAttachModel(api.SenderUser, "hub")
	m.onKey(keyAttach)
	if strings.Contains(m.flash, "user") {
		t.Errorf("the reserved recipient name must never be reported as a missing agent: %q", m.flash)
	}
}
