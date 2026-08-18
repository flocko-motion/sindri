package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestEveryWaitReasonHasWords is the check that makes the split safe. The rule decides WHY a PR
// waits (-> api.PRWaitReason); this front-end only renders it. A reason added there without words
// here fails now, rather than printing a confident remedy for the wrong state — which is how
// merge-failed would have read as "no reviewer is running", sending the reader to start one.
func TestEveryWaitReasonHasWords(t *testing.T) {
	for _, w := range api.PRWaits {
		words, ok := prWaitWords[w]
		if !ok {
			t.Errorf("reason %q has no words: the listing would fall back to a bare 'needs you'", w)
			continue
		}
		if words.row == "" || words.fix == "" {
			t.Errorf("reason %q: row=%q fix=%q — both are shown to a user", w, words.row, words.fix)
		}
		if !strings.Contains(words.fix, "`sindri ") {
			t.Errorf("reason %q's summary should name the command that clears it: %q", w, words.fix)
		}
	}
	if len(prWaitWords) != len(api.PRWaits) {
		t.Errorf("prWaitWords has %d entries for %d reasons — a stale one renders a state that no "+
			"longer exists", len(prWaitWords), len(api.PRWaits))
	}
}

// TestPRWaitRowNamesTheRemedy: the states are cleared by different actions, so the row says which
// one it is. A single "needs you" would send the reader to the wrong verb most of the time.
func TestPRWaitRowNamesTheRemedy(t *testing.T) {
	none := []api.AgentView{{Project: "p", Name: "fili", Role: "worker", Status: "working"}}
	live := []api.AgentView{{Project: "p", Name: "nori", Role: "reviewer", Status: "idle"}}
	cases := []struct {
		pr     api.PR
		agents []api.AgentView
		want   string
	}{
		{api.PR{Project: "p", ID: "pr-1", Status: "approved"}, live, "merge"},
		{api.PR{Project: "p", ID: "pr-2", Status: "open"}, none, "reviewer"},
		{api.PR{Project: "p", ID: "pr-4", Status: "open", Kind: "interim"}, live, "interim"},
		{api.PR{Project: "p", ID: "pr-5", Status: "merge-failed"}, live, "base branch"},
	}
	for _, c := range cases {
		if got := prWaitRow(api.PRWaitReason(c.pr, c.agents)); !strings.Contains(got, c.want) {
			t.Errorf("%s (%s): row said %q, want it to mention %q", c.pr.ID, c.pr.Status, got, c.want)
		}
	}
	// A PR a running reviewer will pick up waits on nobody, so it carries no marker at all.
	if got := prWaitRow(api.PRWaitReason(api.PR{Project: "p", ID: "pr-3", Status: "open"}, live)); got != "" {
		t.Errorf("a PR the queue is moving on should carry no marker, got %q", got)
	}
	// An interim PR must not read "no reviewer is running": starting one would change nothing.
	interim := prWaitRow(api.PRWaitReason(api.PR{Project: "p", Status: "open", Kind: "interim"}, live))
	if strings.Contains(interim, "running") {
		t.Errorf("an interim PR is user-gated, not short of a reviewer: %q", interim)
	}
}

// TestPRNeedsYouSummaryGroupsByWhatToDo: the closing line is read once, so it names the PRs grouped
// by the command that clears each, most stuck first, rather than repeating a verdict per row.
func TestPRNeedsYouSummaryGroupsByWhatToDo(t *testing.T) {
	got := prNeedsYouSummary([]api.PR{
		{Project: "p", ID: "pr-a", Status: "approved"},
		{Project: "p", ID: "pr-b", Status: "open"},
		{Project: "p", ID: "pr-c", Status: "merged"},
		{Project: "p", ID: "pr-d", Status: "rejected"},
		{Project: "p", ID: "pr-e", Status: "open", Kind: "interim"},
		{Project: "p", ID: "pr-f", Status: "merge-failed"},
	}, []api.AgentView{{Project: "p", Name: "fili", Role: "worker", Status: "working"}})

	if !strings.HasPrefix(got, "4 PR(s) need you") {
		t.Errorf("want the four waiting PRs counted, got %q", got)
	}
	for _, want := range []string{"pr-a", "pr-b", "pr-e", "pr-f", "sindri pr merge", "sindri pr approve", "--role reviewer"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary should mention %q: %q", want, got)
		}
	}
	for _, absent := range []string{"pr-c", "pr-d"} {
		if strings.Contains(got, absent) {
			t.Errorf("%s waits on nobody and should be left out: %q", absent, got)
		}
	}
	// Most stuck first: a half-applied merge outranks work that is merely unmerged.
	if strings.Index(got, "pr-f") > strings.Index(got, "pr-a") {
		t.Errorf("the merge-failed PR should be named before the approved one: %q", got)
	}
}

// TestPRNeedsYouSummarySilentWhenTheQueueIsMoving: a call to act, not a status report.
func TestPRNeedsYouSummarySilentWhenTheQueueIsMoving(t *testing.T) {
	if got := prNeedsYouSummary(
		[]api.PR{{Project: "p", ID: "pr-a", Status: "open"}, {Project: "p", ID: "pr-b", Status: "merged"}},
		[]api.AgentView{{Project: "p", Name: "nori", Role: "reviewer", Status: "reviewing"}},
	); got != "" {
		t.Errorf("nothing waiting on the user should print nothing, got %q", got)
	}
}
