package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestPRWaitReasonNamesTheRemedy: the two waiting states are cleared by different actions, so the
// row says which one it is. "Needs you" alone would send the reader to the wrong verb half the time.
func TestPRWaitReasonNamesTheRemedy(t *testing.T) {
	none := []api.AgentView{{Name: "fili", Role: "worker", Status: "working"}}
	live := []api.AgentView{{Name: "nori", Role: "reviewer", Status: "idle"}}
	if got := prWaitReason(api.PR{ID: "pr-1", Status: "approved"}, live); !strings.Contains(got, "merge") {
		t.Errorf("an approved PR waits on the merge, got %q", got)
	}
	if got := prWaitReason(api.PR{ID: "pr-2", Status: "open"}, none); !strings.Contains(got, "reviewer") {
		t.Errorf("an open PR with nobody to review it should say so, got %q", got)
	}
	if got := prWaitReason(api.PR{ID: "pr-3", Status: "open"}, live); got != "" {
		t.Errorf("a PR a running reviewer will pick up waits on nobody, got %q", got)
	}
	// An interim PR is user-gated whatever is running, so its reason must not read "no reviewer" —
	// starting one would change nothing.
	got := prWaitReason(api.PR{ID: "pr-4", Status: "open", Kind: "interim"}, live)
	if !strings.Contains(got, "interim") || strings.Contains(got, "running") {
		t.Errorf("an interim PR should say it is user-gated, got %q", got)
	}
}

// TestPRNeedsYouSummaryGroupsByWhatToDo: the closing line is read once, so it names the PRs and the
// command that clears each group rather than repeating a verdict per row.
func TestPRNeedsYouSummaryGroupsByWhatToDo(t *testing.T) {
	got := prNeedsYouSummary([]api.PR{
		{ID: "pr-a", Status: "approved"},
		{ID: "pr-b", Status: "open"},
		{ID: "pr-c", Status: "merged"},
		{ID: "pr-d", Status: "rejected"},
		{ID: "pr-e", Status: "open", Kind: "interim"},
	}, []api.AgentView{{Name: "fili", Role: "worker", Status: "working"}})

	if !strings.HasPrefix(got, "3 PR(s) need you") {
		t.Errorf("want the three waiting PRs counted, got %q", got)
	}
	for _, want := range []string{"pr-a", "pr-b", "pr-e", "sindri pr merge", "sindri pr approve", "--role reviewer"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary should mention %q: %q", want, got)
		}
	}
	for _, absent := range []string{"pr-c", "pr-d"} {
		if strings.Contains(got, absent) {
			t.Errorf("%s waits on nobody and should be left out: %q", absent, got)
		}
	}
}

// TestPRNeedsYouSummarySilentWhenTheQueueIsMoving: a call to act, not a status report.
func TestPRNeedsYouSummarySilentWhenTheQueueIsMoving(t *testing.T) {
	if got := prNeedsYouSummary(
		[]api.PR{{ID: "pr-a", Status: "open"}, {ID: "pr-b", Status: "merged"}},
		[]api.AgentView{{Name: "nori", Role: "reviewer", Status: "reviewing"}},
	); got != "" {
		t.Errorf("nothing waiting on the user should print nothing, got %q", got)
	}
}
