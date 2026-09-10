package chat

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// recorder is a Delivery that remembers what was typed into each agent, which is the only way to
// assert what a joiner actually READ — the catch-up is a delivery, not a return value.
//
// Locked because a broadcast fans out to members concurrently.
type recorder struct {
	mu        sync.Mutex
	sent      map[string][]string
	whenReady map[string][]string // the subset delivered without interrupting the agent
}

func newRecorder() *recorder {
	return &recorder{sent: map[string][]string{}, whenReady: map[string][]string{}}
}

func (r *recorder) Inject(project, name, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent[name] = append(r.sent[name], text)
	return nil
}

// InjectWhenReady records the same way but keeps the two apart, so a test can say whether an agent
// was interrupted or waited for.
func (r *recorder) InjectWhenReady(project, name, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.whenReady[name] = append(r.whenReady[name], text)
	r.sent[name] = append(r.sent[name], text)
	return nil
}
func (r *recorder) Running(project, name string) bool { return true }
func (r *recorder) Notify()                           {}

// linesTo joins everything delivered to one agent, for substring assertions.
func (r *recorder) linesTo(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.sent[name], "\n")
}

// linesOf copies the individual lines delivered to one agent, so a test can assert on each
// delivery as it was sent — joining them first would hide whether one carried a line break.
func (r *recorder) linesOf(name string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent[name]...)
}

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
	for _, line := range rec.linesOf("nori") {
		if strings.ContainsAny(line, "\n\r") {
			t.Errorf("a delivered line must not carry a line break: %q", line)
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
	for _, line := range rec.linesOf("nori") {
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

// TestIsCommand pins the rule a composer needs to submit on Enter. Exported from here so the TUI
// asks the core rather than carrying a second copy that could drift.
func TestIsCommand(t *testing.T) {
	for _, c := range []struct {
		line string
		want bool
	}{
		{"/who", true},
		{"/add nori", true},
		{"  /remove nori", true}, // leading space: the hub trims before deciding, so this must too
		{"hello room", false},
		{"", false},
		{"tell them / is a slash", false},
	} {
		if got := IsCommand(c.line); got != c.want {
			t.Errorf("IsCommand(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

// TestBroadcastReachesEveryMemberOnce guards the concurrent fan-out: delivery is parallel so the
// sender waits for the slowest agent instead of all of them, and each member must still be typed
// into exactly once — while the sender never receives its own words back.
func TestBroadcastReachesEveryMemberOnce(t *testing.T) {
	s, _, rec := roomWith(t, "dvalin", "nori", "austri")
	for _, n := range []string{"dvalin", "nori", "austri"} {
		if err := s.Add("repo", n); err != nil {
			t.Fatal(err)
		}
	}
	before := map[string]int{}
	for _, n := range []string{"dvalin", "nori", "austri"} {
		before[n] = len(rec.linesOf(n))
	}

	if _, err := s.Say("one message to the room"); err != nil {
		t.Fatal(err)
	}

	for _, n := range []string{"dvalin", "nori", "austri"} {
		got := rec.linesOf(n)
		if len(got)-before[n] != 1 {
			t.Errorf("%s received %d lines for one broadcast, want 1", n, len(got)-before[n])
		}
		if !strings.Contains(got[len(got)-1], "one message to the room") {
			t.Errorf("%s got %q", n, got[len(got)-1])
		}
	}
}

// TestBroadcastSkipsTheSender: an agent's own words must not be typed back into its session.
func TestBroadcastSkipsTheSender(t *testing.T) {
	s, _, rec := roomWith(t, "dvalin", "nori")
	for _, n := range []string{"dvalin", "nori"} {
		if err := s.Add("repo", n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.broadcast("repo", "dvalin", "my own words"); err != nil {
		t.Fatal(err)
	}
	if got := rec.linesTo("dvalin"); strings.Contains(got, "my own words") {
		t.Errorf("the sender must not receive its own message:\n%s", got)
	}
	if got := rec.linesTo("nori"); !strings.Contains(got, "my own words") {
		t.Errorf("the other member should have received it:\n%s", got)
	}
}
