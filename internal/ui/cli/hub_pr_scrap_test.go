package cli

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// fakePRScraper fakes prScraper's three calls, recording each ScrapTask invocation's arguments so
// a test can tell "scrapped alone" from "scrapped with its task" apart.
type fakePRScraper struct {
	pr        api.PRDetail
	prErr     error
	discarded []string
	scrapped  []struct {
		id               string
		subtree, withPRs bool
	}
	discardErr, scrapErr error
}

func (f *fakePRScraper) PRInfo(string) (api.PRDetail, error) { return f.pr, f.prErr }

func (f *fakePRScraper) DiscardPR(id string) error {
	f.discarded = append(f.discarded, id)
	return f.discardErr
}

func (f *fakePRScraper) ScrapTask(id string, subtree, withPRs bool) error {
	f.scrapped = append(f.scrapped, struct {
		id               string
		subtree, withPRs bool
	}{id, subtree, withPRs})
	return f.scrapErr
}

// TestScrapPRAloneNeverTouchesTheTask: the default path is DiscardPR only — PRInfo is never
// called, so a task with no open PR left over is exactly as untouched as before this feature.
func TestScrapPRAloneNeverTouchesTheTask(t *testing.T) {
	var out bytes.Buffer
	f := &fakePRScraper{pr: api.PRDetail{PR: api.PR{Task: "td-1"}, Task: api.Task{ID: "td-1", Status: "open"}}}
	if err := scrapPR(f, "pr-1", false, &out); err != nil {
		t.Fatalf("scrapPR: %v", err)
	}
	if !slices.Equal(f.discarded, []string{"pr-1"}) {
		t.Errorf("discarded = %v, want exactly [pr-1]", f.discarded)
	}
	if len(f.scrapped) != 0 {
		t.Errorf("scrapped = %v, want none — --task was not asked for", f.scrapped)
	}
	if !strings.Contains(out.String(), "branch deleted") {
		t.Errorf("output = %q, want it to say the branch is gone", out.String())
	}
}

// TestScrapPRWithTaskScrapsBothThroughOneCall: withTask=true goes through ScrapTask(withPRs=true)
// alone, the exact path the task-scrap modal's "+ PR" option already takes — DiscardPR must never
// also run, or the PR would be told to vanish twice.
func TestScrapPRWithTaskScrapsBothThroughOneCall(t *testing.T) {
	var out bytes.Buffer
	f := &fakePRScraper{pr: api.PRDetail{PR: api.PR{Task: "td-1"}, Task: api.Task{ID: "td-1", Status: "open"}}}
	if err := scrapPR(f, "pr-1", true, &out); err != nil {
		t.Fatalf("scrapPR: %v", err)
	}
	if len(f.discarded) != 0 {
		t.Errorf("discarded = %v, want none — ScrapTask already scraps this PR", f.discarded)
	}
	if len(f.scrapped) != 1 || f.scrapped[0].id != "td-1" || f.scrapped[0].subtree || !f.scrapped[0].withPRs {
		t.Errorf("scrapped = %v, want one call for td-1 with subtree=false, withPRs=true", f.scrapped)
	}
	if !strings.Contains(out.String(), "td-1 scrapped too") {
		t.Errorf("output = %q, want it to name the task scrapped alongside", out.String())
	}
}

// TestScrapPRWithTaskRefusesASettledTask is the blocking gap this closes: the TUI's modal never
// offers "+ task" once the task is done, so the CLI must refuse the same case rather than
// deleting a finished task's record and its history just because it was asked with --task.
func TestScrapPRWithTaskRefusesASettledTask(t *testing.T) {
	for _, status := range []string{"closed", "merged", "approved"} {
		var out bytes.Buffer
		f := &fakePRScraper{pr: api.PRDetail{PR: api.PR{Task: "td-1"}, Task: api.Task{ID: "td-1", Status: status}}}
		err := scrapPR(f, "pr-1", true, &out)
		if err == nil {
			t.Fatalf("status=%q: scrapPR should have refused, got nil error", status)
		}
		if !strings.Contains(err.Error(), status) {
			t.Errorf("status=%q: error should name it, got %q", status, err.Error())
		}
		if len(f.scrapped) != 0 || len(f.discarded) != 0 {
			t.Errorf("status=%q: a refusal must not touch anything, got scrapped=%v discarded=%v",
				status, f.scrapped, f.discarded)
		}
	}
}

// TestScrapPRWithTaskRefusesNoTask: a PR that names no task has nothing for --task to reach.
func TestScrapPRWithTaskRefusesNoTask(t *testing.T) {
	var out bytes.Buffer
	f := &fakePRScraper{pr: api.PRDetail{PR: api.PR{Task: ""}}}
	if err := scrapPR(f, "pr-1", true, &out); err == nil {
		t.Fatal("scrapPR should have refused a PR with no task")
	}
	if len(f.scrapped) != 0 || len(f.discarded) != 0 {
		t.Errorf("a refusal must not touch anything, got scrapped=%v discarded=%v", f.scrapped, f.discarded)
	}
}

// TestScrapPRWithTaskStopsOnAnUnreadablePR: PRInfo failing must not fall through to scrapping
// blind.
func TestScrapPRWithTaskStopsOnAnUnreadablePR(t *testing.T) {
	var out bytes.Buffer
	f := &fakePRScraper{prErr: errors.New("hub unreachable")}
	if err := scrapPR(f, "pr-1", true, &out); err == nil {
		t.Fatal("scrapPR should have surfaced the PRInfo error")
	}
	if len(f.scrapped) != 0 || len(f.discarded) != 0 {
		t.Errorf("a read failure must not touch anything, got scrapped=%v discarded=%v", f.scrapped, f.discarded)
	}
}
