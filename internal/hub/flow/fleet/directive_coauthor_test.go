// package: hub/flow/fleet / directive_coauthor_test
// type:    logic (what a coauthor is told it may do)
// job:     pin that the review QUEUE never reaches a coauthor, however many verbs it holds — the
// grant is the verbs, and a directive that offered it queued work would make it a reviewer.
// limits:  the directive. What a coauthor's verdict RECORDS is tested beside the acts that write
// it (-> flow/task's planner_act_coauthor_test.go).
package fleet

import (
	"context"
	agentflow "github.com/flo-at/sindri/internal/hub/flow/agent"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/coauthor"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// coauthorEngine seeds a coauthor beside a PR nobody has claimed, which is the shape the queue
// would hand over if it were allowed to.
func coauthorEngine(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	e := storelessEngine(t, &stubDeps{})
	if err := e.Store.RegisterProject("repo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := e.Store.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "brokk", Role: "coauthor", Workspace: ".worktrees/brokk"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "sd-1", Agent: "dvalin", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	// The row the queue would hand over. Unclaimed on purpose: whether it STAYS that way is the
	// whole question, and a claimed one would answer it before the test started.
	if _, err := ps.AddReview("pr-1", "review it"); err != nil {
		t.Fatal(err)
	}
	return e, ps
}

// TestTheQueueNeverHandsACoauthorAReview is the other half of the grant: the VERBS, not the queue.
// A reviewer pulls work and blocks on it; a coauthor reviews when the user asks. If the hub could
// hand it a review, it would be waiting on the fleet instead of on the user.
func TestTheQueueNeverHandsACoauthorAReview(t *testing.T) {
	e, ps := coauthorEngine(t)

	e.LookProject("repo") // every agent re-decides; a coauthor's map offers no route to a review
	if held, _ := ps.ReviewingPR("brokk"); held != "" {
		t.Errorf("the coauthor was handed %s — nothing may assign it a review", held)
	}
	var id int64
	var prID string
	found, err := ps.UnclaimedReview(&id, &prID)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("the review row was claimed by somebody — the only agents here are a coauthor and a worker")
	}
	// And what it is told is unchanged: the user drives it, with an open review sitting right there.
	dir, err := e.AgentDirective(context.Background(), "repo", "brokk")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, prompts.DirCoauthor) {
		t.Errorf("directive = %q, want the coauthor's own — never a review assignment", dir)
	}
}

// TestACoauthorIsNeverToldAnythingUnprompted is the regression for a coauthor sitting quietly with
// its whole role briefing typed at it. `collab` declared Tells on the premise that "the one way a
// coauthor reaches this state again is a session somebody just emptied" — false the moment every
// role gained the full lifecycle: six states lead back to it, so reading one message re-briefed it.
//
// Nothing a coauthor could be told here is news. Its brief is standing in its own system prompt,
// which a cleared session keeps, so the state has nothing to say and says nothing.
func TestACoauthorIsNeverToldAnythingUnprompted(t *testing.T) {
	reached := 0
	for _, s := range coauthor.Flow {
		from := s.Name
		for _, e := range s.Events {
			if e.To != coauthor.Collab {
				continue
			}
			reached++
			landing, found := agentflow.ByName(e.To)
			if !found {
				t.Fatalf("%s leads to undeclared %s", from, e.To)
			}
			if landing.Tells {
				t.Errorf("%s -> %s would push the coauthor's own brief at it for %q", from, e.To, e.Why)
			}
		}
	}
	// The count is the point: one edge back was the premise Tells was written on, and there are six.
	if reached < 5 {
		t.Errorf("only %d edges lead back to collab — the premise this guards may have changed", reached)
	}
}
