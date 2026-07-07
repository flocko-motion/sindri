// package: hub / github_source
// type:    logic (the GitHub issue source + its throttle)
// job:     turn a project's open GitHub issues into cached tasks — the third source
//          merged by SyncTasks — behind a per-project opt-in and a short-TTL memo so
//          the frequent idle-worker resync never hammers the GitHub API.
// limits:  read side only; close-on-merge lives in workflow_merge.go. All GitHub
//          access goes through internal/adapter/github (never the API directly).
package hub

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/adapter/github"
	"github.com/flo-at/sindri/internal/hub/store"
)

// githubTTL throttles the network source: SyncTasks can fire every workPollInterval
// (3s) per idle worker, so we reuse the last issue list within this window and skip
// the gh call. A local td/openspec read is cheap; a gh API call is rate-limited.
const githubTTL = 45 * time.Second

// githubIssueTimeout bounds a single `gh issue list` so a hung network call can't
// stall a sync.
const githubIssueTimeout = 15 * time.Second

// ghCacheEntry is one project's memoized issue list with the moment it was fetched.
type ghCacheEntry struct {
	issues    []github.Issue
	fetchedAt time.Time
}

// ghCache is the hub-side per-project memo (the adapter stays stateless). Guarded by
// ghMu; lazily created so the zero Hub needs no extra init.
var (
	ghMu    sync.Mutex
	ghCache = map[string]ghCacheEntry{}
)

// githubID is the stable task id for a GitHub issue: gh-<number>.
func githubID(number int) string { return "gh-" + strconv.Itoa(number) }

// githubIssueNumber parses a gh-<number> task id back to its issue number,
// reporting ok=false for any non-gh id.
func githubIssueNumber(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, "gh-")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return n, true
}

// githubRows returns the project's open issues as store.Task rows, or nil. It is
// best-effort by contract: a fetch error degrades to no tasks (logged, never
// returned) so a network hiccup can't fail the whole sync. Honors the opt-in and
// the TTL memo; force bypasses the TTL (an explicit user refresh).
func (h *Hub) githubRows(project, root string, enabled, force bool) []store.Task {
	if !enabled || !github.Enabled(root) {
		return nil
	}
	issues, ok := h.githubIssues(project, root, force)
	if !ok {
		return nil
	}
	return issuesToRows(issues)
}

// issuesToRows maps GitHub issues to cached tasks. Issues import UNRATED (no
// priority): they're visible in the backlog but OpenLeaves skips empty-priority
// tasks, so a worker never auto-claims an unvetted issue — a human rates one (via
// the priority override) to release it for assignment, exactly like openspec items.
func issuesToRows(issues []github.Issue) []store.Task {
	rows := make([]store.Task, 0, len(issues))
	for _, is := range issues {
		rows = append(rows, store.Task{
			ID:          githubID(is.Number),
			Title:       is.Title,
			Status:      "open",
			Type:        "issue",
			Priority:    "", // unrated — visible, not auto-claimed until a human rates it
			Description: is.Body,
		})
	}
	return rows
}

// githubIssues returns the project's open issues, served from the TTL memo when
// fresh (no network call) and refetched otherwise. On a fetch error it warns and
// falls back to the last good list until it recovers — a stale error never blanks
// the backlog. Returns ok=false only when there's nothing to contribute at all.
func (h *Hub) githubIssues(project, root string, force bool) ([]github.Issue, bool) {
	ghMu.Lock()
	entry, cached := ghCache[project]
	ghMu.Unlock()
	if cached && !force && time.Since(entry.fetchedAt) < githubTTL {
		return entry.issues, true // cache hit — no network call
	}

	ctx, cancel := context.WithTimeout(context.Background(), githubIssueTimeout)
	defer cancel()
	issues, err := github.Issues(ctx, root)
	if err != nil {
		log.Printf("hub: github issue source for %s degraded (keeping last list): %v", project, err)
		if cached {
			return entry.issues, true // keep the last good list until it recovers
		}
		return nil, false
	}
	ghMu.Lock()
	ghCache[project] = ghCacheEntry{issues: issues, fetchedAt: time.Now()}
	ghMu.Unlock()
	return issues, true
}

// closeGitHubIssue closes+comments the GitHub issue behind a merged gh-* PR. It is a
// no-op for non-gh tasks, and best-effort for gh ones: the local merge already
// landed, so a GitHub failure is logged and recorded on the PR as a warning — never
// returned, never blocking. This is the ONLY outbound GitHub write in the merge path.
func (h *Hub) closeGitHubIssue(project, root string, pr store.PR, prID string) {
	number, ok := githubIssueNumber(pr.Task)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), githubIssueTimeout)
	defer cancel()
	comment := "merged by sindri: " + pr.Branch + " (" + prID + ")"
	if err := github.Close(ctx, root, number, comment); err != nil {
		log.Printf("hub: %s merged locally but closing GitHub issue #%d failed (it stays open upstream): %v", prID, number, err)
		_ = h.store.For(project).LogPR(prID, "warning", "merged locally, but closing GitHub issue failed (stays open upstream): "+err.Error())
		return
	}
	_ = h.store.For(project).LogPR(prID, "github-closed", "closed GitHub issue #"+strconv.Itoa(number))
}
