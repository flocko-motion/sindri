package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// justNow is a timestamp inside the sub-minute window, which is the only window these bugs appear in.
func justNow() string { return time.Now().UTC().Format(time.RFC3339) }

// TestTheMailDetailReadsAsProseWhenJustRead pins the CALL SITE, not the helper: theme.Ago is tested
// in its own package, and reverting this line to Age+" ago" left that suite green.
func TestTheMailDetailReadsAsProseWhenJustRead(t *testing.T) {
	m := mailModel()
	m.state.Mail[0].ReadAt = justNow()
	detail := strings.Join(m.mailDetailLines(), "\n")
	if strings.Contains(detail, "now ago") {
		t.Errorf("the mail detail said %q:\n%s", "now ago", detail)
	}
	if !strings.Contains(detail, "read just now") {
		t.Errorf("a message just read should say so:\n%s", detail)
	}
}

// TestTheRepoDetailReadsAsProseWhenJustUsed: the same composition on the Repos tab, which said
// "used: now ago" for a repo touched this minute.
func TestTheRepoDetailReadsAsProseWhenJustUsed(t *testing.T) {
	m := newModel(nil, nil, "/r/sindri")
	m.tab = 3
	m.state = api.BoardState{Projects: []api.Project{{Tag: "sin", Path: "/r/sindri", LastUsed: justNow()}}}
	m.cursor[3] = 0
	detail := strings.Join(m.repoDetailLines(), "\n")
	if strings.Contains(detail, "now ago") {
		t.Errorf("the repo detail said %q:\n%s", "now ago", detail)
	}
	if !strings.Contains(detail, "used:   just now") {
		t.Errorf("a repo used this minute should say so:\n%s", detail)
	}
}

// TestAStatusJustSetIsNotHeldForNow is the nastier one: "open for now" is real English meaning the
// opposite, so unlike "now ago" it reads as correct while saying something untrue.
func TestAStatusJustSetIsNotHeldForNow(t *testing.T) {
	got := statusHeldFor(api.PR{Status: "open", StatusChangedAt: justNow()})
	if strings.Contains(got, "for now") {
		t.Errorf("statusHeldFor = %q — reads as 'for the time being'", got)
	}
	if !strings.Contains(got, "under a minute") {
		t.Errorf("statusHeldFor = %q, want it to spell the duration out", got)
	}
	// A row predating the column carries no suffix at all: an unadorned status beats one qualified
	// by "n/a", which is the behaviour Held's Unknown exists to let the caller keep.
	if got := statusHeldFor(api.PR{Status: "open"}); got != "" {
		t.Errorf("statusHeldFor with no stamp = %q, want no suffix", got)
	}
}
