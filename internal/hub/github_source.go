// package: hub / github_source
// type:    logic (close-on-merge for GitHub-backed tasks)
// job:     close+comment the upstream GitHub issue when a gh-* PR merges — the one
//          outbound GitHub write in the merge path. The issue task source (fetch,
//          throttle, opt-in) lives entirely in the adapter (adapter/tasks/github);
//          the hub no longer knows how tasks are sourced.
// limits:  one best-effort outbound write; all GitHub access goes through the adapter.
package hub

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/flo-at/sindri/internal/adapter/tasks/github"
	"github.com/flo-at/sindri/internal/hub/store"
)

// githubCloseTimeout bounds the outbound close-on-merge call.
const githubCloseTimeout = 15 * time.Second

// closeGitHubIssue closes+comments the GitHub issue behind a merged gh-* PR. It is a
// no-op for non-gh tasks, and best-effort for gh ones: the local merge already
// landed, so a GitHub failure is logged and recorded on the PR as a warning — never
// returned, never blocking. This is the ONLY outbound GitHub write in the merge path.
func (h *Hub) closeGitHubIssue(project, root string, pr store.PR, prID string) {
	number, ok := github.Number(pr.Task)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), githubCloseTimeout)
	defer cancel()
	comment := "merged by sindri: " + pr.Branch + " (" + prID + ")"
	if err := github.Close(ctx, root, number, comment); err != nil {
		log.Printf("hub: %s merged locally but closing GitHub issue #%d failed (it stays open upstream): %v", prID, number, err)
		_ = h.store.For(project).LogPR(prID, "warning", "merged locally, but closing GitHub issue failed (stays open upstream): "+err.Error())
		return
	}
	_ = h.store.For(project).LogPR(prID, "github-closed", "closed GitHub issue #"+strconv.Itoa(number))
}
