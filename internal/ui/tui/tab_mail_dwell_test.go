package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
)

// findKind returns the first item of the given kind, so a test can assert on it without repeating
// the item's own selection loop.
func findKind(items []metaItem, kind string) (metaItem, bool) {
	for _, it := range items {
		if it.kind == kind {
			return it, true
		}
	}
	return metaItem{}, false
}

// mailToUserModel is the Mail tab with one unread message addressed to the user, selected — the
// case the dwell and its ENTER fallback both exist to mark, wide by default (newModel's w=80 meets
// detailMinWidth).
func mailToUserModel() model {
	m := newModel(&client.HTTP{}, nil, "/r/one") // never dialled: these cases end before any command runs
	m.tab, m.mailFilter = 6, api.MailAll
	m.state = api.BoardState{
		Mail: []api.Mail{{ID: 5, Project: "repo", Repo: "one", Agent: api.SenderUser, Sender: "dvalin",
			Body: "a note", SentAt: "2026-08-13T10:00:00Z"}},
	}
	m.reclamp()
	return m
}

// TestMailDwellFiredRequiresEveryCondition is the scope guarantee and the "resets on the fly" rule
// together: a stale, narrowed, moved-on, already-read or agent-addressed dwell is a no-op, checked
// at the moment it would act rather than when it was armed (-> mailDwellFired).
func TestMailDwellFiredRequiresEveryCondition(t *testing.T) {
	if cmd := mailToUserModel().mailDwellFired(5); cmd == nil {
		t.Error("every condition holds — expected a mark-read cmd")
	}
	wrongTab := mailToUserModel()
	wrongTab.tab = 0
	if cmd := wrongTab.mailDwellFired(5); cmd != nil {
		t.Error("must not fire once the tab has changed")
	}
	narrow := mailToUserModel()
	narrow.w = 40 // below detailMinWidth: no split detail pane
	if cmd := narrow.mailDwellFired(5); cmd != nil {
		t.Error("must not fire without the detail pane showing")
	}
	if cmd := mailToUserModel().mailDwellFired(999); cmd != nil {
		t.Error("must not fire for a message no longer selected")
	}
	alreadyRead := mailToUserModel()
	alreadyRead.state.Mail[0].ReadAt = "2026-08-13T10:05:00Z"
	if cmd := alreadyRead.mailDwellFired(5); cmd != nil {
		t.Error("must not re-fire on an already-read message")
	}
	toAgent := mailToUserModel()
	toAgent.state.Mail = []api.Mail{{ID: 5, Project: "repo", Agent: "dvalin", Sender: "hub", Body: "x"}}
	toAgent.reclamp()
	if cmd := toAgent.mailDwellFired(5); cmd != nil {
		t.Error("must never mark an agent's mail read from the Mail tab")
	}
}

// TestMailSyncCmdsArmsTheDwellOnlyWithTheDetailPaneShowing: a narrow terminal has no split pane for
// a dwell to time against, so only the fetch is scheduled there; the split-pane case schedules a
// dwell alongside it. Checked on the cmd list itself, never by invoking a cmd — one reaches the
// network, the other would block out the real interval.
func TestMailSyncCmdsArmsTheDwellOnlyWithTheDetailPaneShowing(t *testing.T) {
	if got := len(mailSyncCmds(&client.HTTP{}, 5, true)); got != 2 {
		t.Errorf("a shown detail pane should schedule fetch + dwell, got %d cmd(s)", got)
	}
	if got := len(mailSyncCmds(&client.HTTP{}, 5, false)); got != 1 {
		t.Errorf("no detail pane showing should schedule the fetch alone, got %d cmd(s)", got)
	}
}

// TestNarrowEnterMarksTheUsersMailReadAsTheDwellFallback: on a narrow terminal ENTER is the body's
// first appearance (the split pane is what a dwell times against everywhere else), so it plays the
// dwell's role there instead.
func TestNarrowEnterMarksTheUsersMailReadAsTheDwellFallback(t *testing.T) {
	m := mailToUserModel()
	m.w = 40
	m.reclamp()
	cmd := m.onKey("enter")
	if cmd == nil {
		t.Error("ENTER on a narrow terminal should mark the user's unread mail read")
	}
	if !m.modal {
		t.Error("ENTER should still open the detail modal")
	}
}

// TestEnterMarksReadWithBothPanesShowing: ENTER on a message IS reading it, whatever else is on
// screen. It used to mark read only where the detail pane was hidden, so opening one deliberately
// beside a visible pane left it unread until the three-second dwell caught up — a wait for something
// the user had already done.
func TestEnterMarksReadWithBothPanesShowing(t *testing.T) {
	m := mailToUserModel()
	if cmd := m.onKey("enter"); cmd == nil {
		t.Error("ENTER should mark the user's unread mail read, pane or no pane")
	}
	if !m.modal {
		t.Error("ENTER should still open the detail modal")
	}
}

// mailBodyModel is the Mail tab with one message from a real agent, long enough that its body
// would wrap across several lines — the case a per-line kind would make the cursor walk through.
func mailBodyModel() model {
	m := newModel(&client.HTTP{}, nil, "/r/one")
	m.tab, m.w, m.h, m.mailFilter = 6, 120, 40, api.MailAll
	m.state = api.BoardState{
		Agents: []api.AgentView{{Name: "dvalin", Project: "repo"}, {Name: "nori", Project: "repo"}},
		Mail: []api.Mail{{ID: 7, Project: "repo", Repo: "one", Agent: "nori", Sender: "dvalin",
			Body: "line one\nline two\nline three", SentAt: "2026-08-19T10:00:00Z"}},
	}
	m.reclamp()
	return m
}

// TestMailBodyIsOneYankableItemNotOnePerLine is sd-c31664: the body must be reachable and
// yankable as ONE field — a per-line kind would make the cursor walk every line of a long
// message just to get past it.
func TestMailBodyIsOneYankableItemNotOnePerLine(t *testing.T) {
	m := mailBodyModel()
	act := m.mailActionable()
	body, ok := findKind(act, "mailbody")
	if !ok {
		t.Fatal("the body should be one of the actionable items")
	}
	if body.value != "line one\nline two\nline three" {
		t.Errorf("the body's value should be the message's own text, got %q", body.value)
	}
	n := 0
	for _, it := range act {
		if it.kind == "mailbody" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected exactly one mailbody item however many lines the message wraps to, got %d", n)
	}
}

// TestMailBodyValueDropsTheFetchingNotice: mailBodyOf appends "(fetching the rest)" to a
// truncated preview for the PANE to show — that's chrome, not the message, and must not end up
// in what `y` copies.
func TestMailBodyValueDropsTheFetchingNotice(t *testing.T) {
	m := mailBodyModel()
	m.state.Mail[0].Body, m.state.Mail[0].Truncated = "only the opening", true
	body, ok := findKind(m.mailActionable(), "mailbody")
	if !ok {
		t.Fatal("the body should be one of the actionable items")
	}
	if strings.Contains(body.value, "fetching") {
		t.Errorf("the yankable value must not carry the pane's own notice: %q", body.value)
	}
	if body.value != "only the opening" {
		t.Errorf("value = %q, want the raw preview until the fetch lands", body.value)
	}
	if !strings.Contains(body.text, "fetching") {
		t.Errorf("the rendered text should still say more is coming: %q", body.text)
	}
}

// fromItem/toItem return the "from:"/"to:" line out of a Mail detail's items — kind varies (only a
// real agent is traceable), so each is found by its text prefix rather than by kind.
func fromItem(items []metaItem) (metaItem, bool) { return itemWithPrefix(items, "from:") }
func toItem(items []metaItem) (metaItem, bool)   { return itemWithPrefix(items, "to:") }

func itemWithPrefix(items []metaItem, prefix string) (metaItem, bool) {
	for _, it := range items {
		if strings.HasPrefix(it.text, prefix) {
			return it, true
		}
	}
	return metaItem{}, false
}

// TestMailFromIsTraceableOnlyForARealAgent: hub/user/reviewer aren't reachable on the Agents tab,
// so only a genuine agent sender is given the "agent" kind.
func TestMailFromIsTraceableOnlyForARealAgent(t *testing.T) {
	m := mailBodyModel() // sender "dvalin" is a real agent in this fixture
	from, ok := fromItem(m.mailItems())
	if !ok || from.kind != "agent" || from.value != "dvalin" {
		t.Errorf("from should be a traceable agent cross-reference, got %+v (ok=%v)", from, ok)
	}

	m.state.Mail[0].Sender = "hub"
	from, ok = fromItem(m.mailItems())
	if !ok || from.kind != "" {
		t.Errorf("hub is not traceable — from must carry no kind, got %+v", from)
	}
}

// TestMailToIsTraceableOnlyForARealAgent is sd-fffd47's own regression: `to` is "user" on every
// message addressed to the user — the traffic this whole epic is about — so it needs the exact
// same guard `from` already had, not a bare kind that treats the reserved name as an agent.
func TestMailToIsTraceableOnlyForARealAgent(t *testing.T) {
	m := mailBodyModel() // recipient "nori" is a real agent in this fixture
	to, ok := toItem(m.mailItems())
	if !ok || to.kind != "agent" || to.value != "nori" {
		t.Errorf("to should be a traceable agent cross-reference, got %+v (ok=%v)", to, ok)
	}

	m.state.Mail[0].Agent = api.SenderUser
	to, ok = toItem(m.mailItems())
	if !ok || to.kind != "" {
		t.Errorf("the reserved recipient \"user\" must not be reported as a traceable agent, got %+v", to)
	}
}
