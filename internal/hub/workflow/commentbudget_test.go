package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
)

// TestCommentBudgetDefaultsToTheLintersOwnCeiling: an unconfigured project resolves to the same
// numbers the gate falls back to — 2.0 the ceiling, 1.5 the aim lint.AimFor derives from it.
func TestCommentBudgetDefaultsToTheLintersOwnCeiling(t *testing.T) {
	e := newEngine(nil, &stubDeps{})
	aim, ceiling := e.commentBudget("repo")
	if ceiling != 2.0 {
		t.Errorf("ceiling = %v, want the linter's own default 2.0", ceiling)
	}
	if aim != 1.5 {
		t.Errorf("aim = %v, want 1.5 (2.0 - 0.5)", aim)
	}
}

// TestCommentBudgetHonorsAProjectOverride: a repo's own lint.max_comment_avg is the SAME override
// repoLintBar reads for the gate, so the brief must resolve it identically rather than the default.
func TestCommentBudgetHonorsAProjectOverride(t *testing.T) {
	override := 3.0
	e := newEngine(nil, &stubDeps{projectConfig: config.Config{Lint: api.Lint{MaxCommentAvg: &override}}})
	aim, ceiling := e.commentBudget("repo")
	if ceiling != 3.0 {
		t.Errorf("ceiling = %v, want the project's own override 3.0", ceiling)
	}
	if aim != 2.5 {
		t.Errorf("aim = %v, want 2.5 (3.0 - 0.5)", aim)
	}
}

// TestCommentBudgetIgnoresAnUnreadableConfig: an unreadable project config must fall back to the
// same default the gate itself falls back to, not error out the whole directive over it.
func TestCommentBudgetIgnoresAnUnreadableConfig(t *testing.T) {
	e := newEngine(nil, &stubDeps{projectConfigErr: errors.New("config unreadable")})
	aim, ceiling := e.commentBudget("repo")
	if ceiling != 2.0 || aim != 1.5 {
		t.Errorf("aim=%v ceiling=%v, want the default (1.5, 2.0) when config can't be read", aim, ceiling)
	}
}

// TestCommentBudgetNoteStatesBothNumbersAsATrend: a per-comment reading of the aim writes worse
// comments to hit it, so the note must say MEAN, name the ceiling too, and the line-width cap.
func TestCommentBudgetNoteStatesBothNumbersAsATrend(t *testing.T) {
	note := CommentBudgetNote(1.5, 2.0)
	for _, want := range []string{"1.5", "2.0", "MEAN", "trend", "110"} {
		if !strings.Contains(note, want) {
			t.Errorf("CommentBudgetNote = %q, want it to contain %q", note, want)
		}
	}
}

// TestClaimedTaskDirectiveCarriesTheProjectsOwnBudget is the end-to-end path: a worker already
// holding its task is told the SAME numbers the submit gate will hold it to, on that project — the
// answer every time it asks (-> DirWorking), not the one-off claim message.
func TestClaimedTaskDirectiveCarriesTheProjectsOwnBudget(t *testing.T) {
	override := 4.0
	deps := &stubDeps{projectConfig: config.Config{Lint: api.Lint{MaxCommentAvg: &override}}}
	e, ps := idleWorkerWithOpenTask(t, deps)
	// The first ask claims the task (-> DirClaimed, a one-off with no budget note); the SECOND is
	// where DirWorking answers every time until it submits.
	if _, err := e.AgentDirective(context.Background(), "repo", "dvalin"); err != nil {
		t.Fatalf("AgentDirective (claim): %v", err)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" || st.Phase != "working" {
		t.Fatalf("precondition: want td-abc123 claimed and working, got %+v", st)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	for _, want := range []string{"3.5", "4.0"} {
		if !strings.Contains(dir, want) {
			t.Errorf("directive = %q, want it to carry the project's own budget (%s)", dir, want)
		}
	}
}
