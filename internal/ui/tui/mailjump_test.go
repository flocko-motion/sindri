package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// mailJumpBoard is two repos: the active one, and a foreign agent with unread mail of its own —
// the case where the destination's repo scope decides whether the jump lands on anything.
func mailJumpBoard() api.BoardState {
	return api.BoardState{
		Projects: []api.Project{
			{Tag: "here", Path: "/r/here"},
			{Tag: "there", Path: "/r/there"},
		},
		Agents: []api.AgentView{
			{Project: "here", Repo: "here", Name: "nori", Role: "worker", Status: "idle", UnreadMail: 2},
			{Project: "there", Repo: "there", Name: "dwalin", Role: "worker", Status: api.StatusBlocked, UnreadMail: 3},
		},
		Mail: []api.Mail{
			{ID: 1, Agent: "nori", Project: "here", Sender: "hub", Body: "unread one"},
			{ID: 2, Agent: "nori", Project: "here", Sender: "hub", Body: "unread two"},
			{ID: 3, Agent: "nori", Project: "here", Sender: "hub", Body: "already read", ReadAt: "2026-08-18T07:00:00Z"},
			{ID: 4, Agent: "dwalin", Project: "there", Sender: "hub", Body: "foreign one"},
			{ID: 5, Agent: "dwalin", Project: "there", Sender: "hub", Body: "foreign two"},
			{ID: 6, Agent: "dwalin", Project: "there", Sender: "hub", Body: "foreign three"},
		},
	}
}

// mailJumpModel is the Agents tab with the cursor on agent, its detail focused. The scope is set
// BEFORE the selection: it decides which rows exist, so selecting first and scoping after would
// leave the cursor on whichever agent had taken that index.
func mailJumpModel(agent string, scopeRepo bool) model {
	m := newModel(nil, nil, "/r/here")
	m.tab, m.w, m.h = 1, 120, 40
	m.state = mailJumpBoard()
	m.scopeRepo = scopeRepo
	m.reclamp()
	m.selectRow(agent)
	m.focus = focusItems
	return m
}

// unreadItem is the agent detail's unread-mail line, and whether it is there at all.
func unreadItem(m model) (metaItem, bool) {
	for _, it := range m.agentItems() {
		if strings.Contains(it.text, "unread") {
			return it, true
		}
	}
	return metaItem{}, false
}

// TestTheUnreadCountIsReachable is the gap: the line stated a fact the cursor could not reach, so
// the number was the beginning of a question the UI would not answer.
func TestTheUnreadCountIsReachable(t *testing.T) {
	m := mailJumpModel("nori", false)
	it, ok := unreadItem(m)
	if !ok {
		t.Fatal("precondition: an agent with unread mail shows the count")
	}
	if it.kind == "" {
		t.Error("the count carries no kind, so the cursor cannot reach it")
	}
	if it.value != "nori" {
		t.Errorf("the item should name the agent whose mail it counts, got %q", it.value)
	}
	var focusable bool
	for _, a := range m.agentActionable() {
		if a.kind == it.kind {
			focusable = true
		}
	}
	if !focusable {
		t.Error("the count is not in the focusable set")
	}
}

// TestNoMailNoItem: the line only exists where there is unread mail, so nothing new is focusable on
// an agent that has read everything.
func TestNoMailNoItem(t *testing.T) {
	m := mailJumpModel("nori", false)
	for i := range m.state.Agents {
		m.state.Agents[i].UnreadMail = 0
	}
	if _, ok := unreadItem(m); ok {
		t.Error("an agent with nothing unread should show no mail line")
	}
}

// TestTheJumpLandsOnExactlyWhatWasCounted is the contradiction the subtask names: a line saying two
// that lands on a list of four is a UI arguing with itself in two keystrokes. Both axes are set —
// recipient and unread — so the destination is the same question asked of the same set.
func TestTheJumpLandsOnExactlyWhatWasCounted(t *testing.T) {
	m := mailJumpModel("nori", false)
	it, _ := unreadItem(m)
	m.gotoItem(it.kind, it.value)

	if m.tab != 6 {
		t.Fatalf("the jump should land on the Mail tab, got tab %d", m.tab)
	}
	shown := m.mailShown()
	if len(shown) != 2 {
		t.Errorf("the count said 2 unread; the destination shows %d rows", len(shown))
	}
	for _, msg := range shown {
		if msg.Agent != "nori" {
			t.Errorf("a message for %q leaked into the narrowed view", msg.Agent)
		}
		if msg.Read() {
			t.Errorf("message %d is read and should not be in an unread view", msg.ID)
		}
	}
}

// TestAJumpFromAForeignAgentShowsItsMail is the scope trap: the count belongs to the AGENT, so a
// destination scoped to the local repo would be empty and read as "no mail" rather than
// "wrong repo" — the jump would be worse than none.
func TestAJumpFromAForeignAgentShowsItsMail(t *testing.T) {
	// The ordinary state: the user is looking at their own repo, and the foreign agent is listed
	// because it is stuck on them.
	m := mailJumpModel("dwalin", true)
	it, ok := unreadItem(m)
	if !ok {
		t.Fatal("precondition: the foreign agent shows its unread count")
	}
	m.gotoItem(it.kind, it.value)

	shown := m.mailShown()
	if len(shown) != 3 {
		t.Fatalf("the count said 3 unread for a foreign agent; the destination shows %d", len(shown))
	}
	for _, msg := range shown {
		if msg.Agent != "dwalin" {
			t.Errorf("widening the scope let %q's mail in; the recipient filter should pin it", msg.Agent)
		}
	}
	if !strings.Contains(m.flash, "repo") {
		t.Errorf("changing the scope under the user should be said out loud, got %q", m.flash)
	}
}

// TestALocalJumpLeavesTheScopeAlone: scope is TUI-wide, so widening it is a side effect worth
// avoiding where the answer is already in view.
func TestALocalJumpLeavesTheScopeAlone(t *testing.T) {
	m := mailJumpModel("nori", true)
	it, _ := unreadItem(m)
	m.gotoItem(it.kind, it.value)
	if !m.scopeRepo {
		t.Error("the agent's mail was already in scope, so the scope should not have moved")
	}
}

// TestEnterOnTheCountJumps is the binding as a user meets it: the item is focused in the detail
// column and ⏎ is pressed. Routed through onKey rather than gotoItem directly, since the enter
// dispatch is a switch on kind and an unlisted one falls through to the details modal instead.
func TestEnterOnTheCountJumps(t *testing.T) {
	m := mailJumpModel("nori", false)
	it, _ := unreadItem(m)
	for i, a := range m.agentActionable() { // put the cursor on the mail item itself
		if a.kind == it.kind {
			m.rightCursor = i
		}
	}
	m.onKey("enter")

	if m.tab != 6 {
		t.Fatalf("⏎ on the unread count should open the Mail tab, got tab %d", m.tab)
	}
	if m.modal {
		t.Error("it opened the details modal instead of jumping — the kind is unhandled in the dispatch")
	}
	if m.mailAgent != "nori" || m.mailFilter != api.MailUnread {
		t.Errorf("both axes should be set, got agent=%q filter=%q", m.mailAgent, m.mailFilter)
	}
}

// windowedMailBoard is the shape the fixtures above cannot express: the count is taken over the
// WHOLE mailbox and the board carries only its newest slice, so some of the agent's unread mail
// sits outside m.state.Mail. It is the target case rather than a corner one — an agent that has
// stopped reading is the one whose unread mail is oldest, and so the first to fall out of the
// window that a count on it exists to surface.
func windowedMailModel() model {
	m := mailJumpModel("nori", false)
	for i, a := range m.state.Agents {
		if a.Name == "nori" {
			m.state.Agents[i].UnreadMail = 5 // the mailbox holds five; the window carries two of them
		}
	}
	m.state.MailTotal = 431
	return m
}

// TestTheJumpSaysWhenTheWindowHidesSomeOfWhatWasCounted is the contradiction in its other direction:
// a line saying five that lands on two rows disagrees with itself exactly as one saying two that
// lands on four does. The destination cannot hold the missing rows, so it must name them.
func TestTheJumpSaysWhenTheWindowHidesSomeOfWhatWasCounted(t *testing.T) {
	m := windowedMailModel()
	it, ok := unreadItem(m)
	if !ok {
		t.Fatal("precondition: the agent shows its unread count")
	}
	m.gotoItem(it.kind, it.value)

	if got := len(m.mailShown()); got != 2 {
		t.Fatalf("precondition: the window should hold 2 of the 5 counted, got %d", got)
	}
	line := m.filterLine()
	for _, want := range []string{"2 of 5 shown", "window"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line should say %q, so the reader is not left counting rows against a number that disagrees; got %q", want, line)
		}
	}
}

// TestAJumpTheWindowHoldsInFullSaysNothingExtra: the note is owed only where rows are missing, and a
// permanent "2 of 2 shown" would be a caveat about nothing.
func TestAJumpTheWindowHoldsInFullSaysNothingExtra(t *testing.T) {
	m := mailJumpModel("nori", false)
	it, _ := unreadItem(m)
	m.gotoItem(it.kind, it.value)
	if strings.Contains(m.filterLine(), "shown") {
		t.Errorf("the window holds every counted message, so nothing is missing to report: %q", m.filterLine())
	}
}

// TestTheShortfallBelongsToTheJumpAlone: the promise was made by a count the user followed. Once
// they move an axis themselves they are asking their own question, and a stale "2 of 5" would be
// answering the one before it.
func TestTheShortfallBelongsToTheJumpAlone(t *testing.T) {
	for name, moved := range map[string]func(*model){
		"cycling the filter": (*model).cycleMailFilter,
		"clearing":           (*model).clearFilters,
		"narrowing by hand":  (*model).cycleMailWho,
	} {
		m := windowedMailModel()
		it, _ := unreadItem(m)
		m.gotoItem(it.kind, it.value)
		if !strings.Contains(m.filterLine(), "of 5") {
			t.Fatalf("%s: precondition: the jump reports the shortfall", name)
		}
		moved(&m)
		if strings.Contains(m.filterLine(), "of 5") {
			t.Errorf("%s left the jump's promise on the line: %q", name, m.filterLine())
		}
	}
}

// TestTheCountOnAForeignAgentIsPromisedToo: widening the scope is the other way a jump's destination
// can differ from its count, and the two corrections must not cancel each other out.
func TestTheCountOnAForeignAgentIsPromisedToo(t *testing.T) {
	m := mailJumpModel("dwalin", true)
	for i, a := range m.state.Agents {
		if a.Name == "dwalin" {
			m.state.Agents[i].UnreadMail = 4 // one more than the window carries
		}
	}
	it, _ := unreadItem(m)
	m.gotoItem(it.kind, it.value)
	if !strings.Contains(m.filterLine(), "3 of 4 shown") {
		t.Errorf("the shortfall should survive the scope widening, got %q", m.filterLine())
	}
	if !strings.Contains(m.flash, "repo") {
		t.Errorf("and the scope change is still said out loud, got %q", m.flash)
	}
}
