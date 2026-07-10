// package: hub / comments
// type:    logic (unified task-comment sync)
// job:     keep a task's comment thread in the hub store fresh from its source —
//          td comments for td-* tasks, GitHub issue comments for gh-*. Reconciles
//          by re-fetching the source's current set (store.ReplaceComments), TTL-
//          throttled so a view is cheap, with a forced path for the refresh key.
// limits:  td-*/gh-* only (os-* has no comments); fetching in the adapters,
//          persistence in store/comment.go, rendering in the UIs.
package hub

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/adapter/github"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/hub/store"
)

// commentTTL throttles comment re-fetches: viewing a task re-syncs its comments at
// most this often (a GitHub call is slow/rate-limited; comments change slowly). The
// refresh key bypasses it.
const commentTTL = time.Hour

// taskComments returns a task's comments for a detail view: the cached set, after a
// re-sync if it's stale. The sync is synchronous but TTL-gated to once an hour, and
// TaskInfo runs in a background command on the UI side — so a view stays responsive
// while still showing current comments. td-*/gh- only.
func (h *Hub) taskComments(project, id string) []store.Comment {
	if h.commentsDue(project, id) {
		if err := h.syncTaskComments(project, id, false); err != nil {
			fmt.Fprintf(os.Stderr, "hub: sync comments for %s: %v\n", id, err)
		}
	}
	cs, err := h.store.For(project).Comments(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hub: read comments for %s: %v\n", id, err)
	}
	return cs
}

// RefreshTaskComments forces a comment re-sync for one task (the [r]efresh key),
// bypassing the TTL, and notifies watchers.
func (h *Hub) RefreshTaskComments(project, id string) error {
	if err := h.syncTaskComments(project, id, true); err != nil {
		return err
	}
	h.notify()
	return nil
}

// syncTaskComments fetches the task's comments from its source and reconciles the
// store against them (add/drop/update). A no-op when not due (unless forced) or for
// a source without comments (os-*). Records the sync time so the TTL holds.
func (h *Hub) syncTaskComments(project, id string, force bool) error {
	if !force && !h.commentsDue(project, id) {
		return nil
	}
	root := h.projectRoot(project)
	var (
		source   string
		comments []store.Comment
	)
	switch {
	case strings.HasPrefix(id, "td-"):
		source = "td"
		tc, err := td.Comments(root, id)
		if err != nil {
			return err
		}
		for _, c := range tc {
			comments = append(comments, store.Comment{
				Source: source, SourceRef: c.ID, Author: c.Author, Body: c.Body,
				CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
	case strings.HasPrefix(id, "gh-"):
		source = "github"
		n, ok := githubIssueNumber(id)
		if !ok {
			return fmt.Errorf("%s: not a valid GitHub id", id)
		}
		ctx, cancel := context.WithTimeout(context.Background(), githubIssueTimeout)
		defer cancel()
		gc, err := github.IssueComments(ctx, root, n)
		if err != nil {
			return err
		}
		for _, c := range gc {
			comments = append(comments, store.Comment{
				Source: source, SourceRef: c.URL, Author: c.Author.Login, Body: c.Body,
				CreatedAt: c.CreatedAt,
			})
		}
	default:
		return nil // os-* and the like carry no comments
	}
	if err := h.store.For(project).ReplaceComments(id, source, comments); err != nil {
		return err
	}
	h.markCommentsSynced(project, id)
	return nil
}

// commentsDue reports whether a task's comments are stale (never synced, or older
// than commentTTL).
func (h *Hub) commentsDue(project, id string) bool {
	h.commentMu.Lock()
	defer h.commentMu.Unlock()
	last, ok := h.commentSynced[project+"\x00"+id]
	return !ok || time.Since(last) >= commentTTL
}

// markCommentsSynced stamps a task's comments as freshly synced.
func (h *Hub) markCommentsSynced(project, id string) {
	h.commentMu.Lock()
	defer h.commentMu.Unlock()
	if h.commentSynced == nil {
		h.commentSynced = map[string]time.Time{}
	}
	h.commentSynced[project+"\x00"+id] = time.Now()
}
