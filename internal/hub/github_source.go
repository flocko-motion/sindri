// package: hub / github_source
// type:    logic (the GitHub issue source throttle + close-on-merge)
// job:     the hub-side policy around the github task source — a per-project opt-in
//          (in SyncTasks) and a short-TTL memo so the frequent idle-worker resync
//          never hammers the GitHub API — plus close-on-merge for a merged gh-* PR.
//          The fetch + issue→task mapping live in the adapter (adapter/tasks/github).
// limits:  read throttle + one outbound close; all GitHub access goes through the
//          adapter, never the API directly.
package hub

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/adapter/tasks/github"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// githubTTL throttles the network source: SyncTasks can fire often (every
// workPollInterval per idle worker, and on task mutations), so we reuse the last
// fetch within this window and skip the gh call. Scanning for new issues every
// couple of minutes is plenty — a local td/openspec read is cheap, a gh API call is
// slow and rate-limited. The [r]efresh hotkey (ForceSyncTasks) bypasses this.
const githubTTL = 2 * time.Minute

// githubCloseTimeout bounds the outbound close-on-merge call.
const githubCloseTimeout = 15 * time.Second

// ghCacheEntry is one project's memoized issue-tasks with the moment it was fetched.
type ghCacheEntry struct {
	tasks     []task.Task
	fetchedAt time.Time
}

// ghCache is the hub-side per-project memo (the adapter stays stateless). Guarded by
// ghMu; lazily created so the zero Hub needs no extra init.
var (
	ghMu    sync.Mutex
	ghCache = map[string]ghCacheEntry{}
)

// githubTasks returns the project's open issues as tasks, served from the TTL memo
// when fresh (no network call) and refetched otherwise via the github source. On a
// fetch error it warns and falls back to the last good list until it recovers — a
// stale error never blanks the backlog. Returns nil when the source isn't usable.
// force bypasses the TTL (an explicit user refresh).
func (h *Hub) githubTasks(project, root string, force bool) []task.Task {
	if !(github.Source{}).Enabled(root) {
		return nil
	}
	ghMu.Lock()
	entry, cached := ghCache[project]
	ghMu.Unlock()
	if cached && !force && time.Since(entry.fetchedAt) < githubTTL {
		return entry.tasks // cache hit — no network call
	}
	ts, err := (github.Source{}).Tasks(root)
	if err != nil {
		log.Printf("hub: github issue source for %s degraded (keeping last list): %v", project, err)
		if cached {
			return entry.tasks // keep the last good list until it recovers
		}
		return nil
	}
	ghMu.Lock()
	ghCache[project] = ghCacheEntry{tasks: ts, fetchedAt: time.Now()}
	ghMu.Unlock()
	return ts
}

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
