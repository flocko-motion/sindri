package chat

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// recorder is a Delivery that remembers what was typed into each agent, which is the only way to
// assert what a joiner actually READ — the catch-up is a delivery, not a return value.
type recorder struct {
	sent map[string][]string
}

func newRecorder() *recorder { return &recorder{sent: map[string][]string{}} }

func (r *recorder) Inject(project, name, text string) error {
	r.sent[name] = append(r.sent[name], text)
	return nil
}
func (r *recorder) Running(project, name string) bool { return true }
func (r *recorder) Notify()                           {}

// linesTo joins everything delivered to one agent, for substring assertions.
func (r *recorder) linesTo(name string) string { return strings.Join(r.sent[name], "\n") }

// roomWith opens a store-backed service with agents present (so Add accepts them).
func roomWith(t *testing.T, names ...string) (*Service, *store.Store, *recorder) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for _, n := range names {
		if err := st.For("repo").PutAgent(store.Agent{Name: n, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	rec := newRecorder()
	return New(st, rec), st, rec
}

// TestNewMeetingClearsHistoryButKeepsMembers: the reset is about what the room remembers, not who
// is in it — dropping the roster would evict every agent and leave the user re-adding them.
func TestNewMeetingClearsHistoryButKeepsMembers(t *testing.T) {
	s, st, _ := roomWith(t, "dvalin")
	if err := s.Add("repo", "dvalin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Say("something from the old meeting"); err != nil {
		t.Fatal(err)
	}

	if err := s.NewMeeting(); err != nil {
		t.Fatalf("NewMeeting: %v", err)
	}

	log, err := st.ChatTranscript(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range log {
		if strings.Contains(m.Body, "old meeting") {
			t.Errorf("history survived the reset: %q", m.Body)
		}
	}
	// Not silence: the announcement is the only thing left, so the room reads as freshly started.
	if len(log) != 1 || !strings.Contains(log[0].Body, "new meeting started") {
		t.Errorf("expected just the fresh-start announcement, got %+v", log)
	}
	members, err := st.ChatMembers()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].Name != "dvalin" {
		t.Errorf("members must survive a new meeting, got %+v", members)
	}
}

// TestNewMeetingTellsTheRoom: members hold the old discussion in their own context, so a silent
// reset would leave them answering a meeting that no longer exists.
func TestNewMeetingTellsTheRoom(t *testing.T) {
	s, _, rec := roomWith(t, "dvalin")
	if err := s.Add("repo", "dvalin"); err != nil {
		t.Fatal(err)
	}
	if err := s.NewMeeting(); err != nil {
		t.Fatal(err)
	}
	if got := rec.linesTo("dvalin"); !strings.Contains(got, "new meeting started") {
		t.Errorf("the room should be told the slate is clean, got:\n%s", got)
	}
}

// TestJoinerIsCaughtUp is the second half of the feature: without the transcript a newcomer
// restarts a discussion the room already had.
func TestJoinerIsCaughtUp(t *testing.T) {
	s, _, rec := roomWith(t, "dvalin", "nori")
	if err := s.Add("repo", "dvalin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Say("the question is whether to fold the walks"); err != nil {
		t.Fatal(err)
	}

	if err := s.Add("repo", "nori"); err != nil { // joins after the discussion started
		t.Fatal(err)
	}

	got := rec.linesTo("nori")
	if !strings.Contains(got, "Catching you up") {
		t.Errorf("a joiner should be caught up, got:\n%s", got)
	}
	if !strings.Contains(got, "fold the walks") {
		t.Errorf("the catch-up must carry what was said, got:\n%s", got)
	}
	if !strings.Contains(got, MsgWelcome) {
		t.Errorf("the welcome should still arrive, got:\n%s", got)
	}
	// One line per delivery: a newline would submit each fragment as its own prompt.
	for _, line := range rec.sent["nori"] {
		if strings.Contains(line, "\n") {
			t.Errorf("a delivered line must not contain a newline: %q", line)
		}
	}
}

// TestJoinerGetsNoCatchUpInAFreshRoom: an empty room has nothing to catch up on, and a "here is
// the history" line with no history reads as a bug.
func TestJoinerGetsNoCatchUpInAFreshRoom(t *testing.T) {
	s, _, rec := roomWith(t, "dvalin")
	if err := s.Add("repo", "dvalin"); err != nil {
		t.Fatal(err)
	}
	if got := rec.linesTo("dvalin"); strings.Contains(got, "Catching you up") {
		t.Errorf("nothing had been said, so there is nothing to catch up on, got:\n%s", got)
	}
}

// TestCatchUpIsBounded: the transcript can outgrow what should be typed into a session, so the
// catch-up drops the OLDEST and says how many — what was said last is what a newcomer is asked about.
func TestCatchUpIsBounded(t *testing.T) {
	s, _, rec := roomWith(t, "dvalin", "nori")
	if err := s.Add("repo", "dvalin"); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 900)
	for i := 0; i < 8; i++ { // ~7200 chars, comfortably past the per-message cap
		if _, err := s.Say(long); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Say("the newest thing said"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("repo", "nori"); err != nil {
		t.Fatal(err)
	}

	var catchUp string
	for _, line := range rec.sent["nori"] {
		if strings.Contains(line, "Catching you up") {
			catchUp = line
		}
	}
	if catchUp == "" {
		t.Fatal("expected a catch-up line")
	}
	if len([]rune(catchUp)) > maxLen {
		t.Errorf("catch-up is %d runes, over the %d cap", len([]rune(catchUp)), maxLen)
	}
	if !strings.Contains(catchUp, "newest thing said") {
		t.Error("the newest message must survive the bound — it is the most relevant")
	}
	if !strings.Contains(catchUp, "omitted for length") {
		t.Error("a trimmed catch-up must admit what it dropped")
	}
}
