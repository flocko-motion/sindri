// package: hub/comments / service
// type:    logic (unified task-comment sync — a hub module, not an adapter)
// job:     keep a task's comment thread fresh in the store from its source — GitHub
// issue comments for gh-*. Reconciles by re-fetching the source's current set
// (store.ReplaceComments), TTL-throttled so a view is cheap, with a forced path
// for the refresh key.
// limits:  gh-* only; a task sindri owns holds its comments here already, and os-*
// carries none. The hub wires ProjectRoot/Notify via a small seam.
package comments

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/adapter/tasks/github"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// ttl throttles re-fetches: a view re-syncs a task's comments at most this often (a
// GitHub call is slow; comments change slowly). The refresh path bypasses it.
const ttl = time.Hour

// githubTimeout bounds a single `gh issue view` so a hung network call can't stall
// a comment sync.
const githubTimeout = 15 * time.Second

// Deps is the seam back to the hub: resolve a project's root path and wake watchers.
// (Not a hexagonal port — just what this module needs from its parent, kept as an
// interface so the module builds and tests independently.)
type Deps interface {
	ProjectRoot(project string) string
	Notify()
}

// Service syncs and serves task comments. Construct with New; the hub owns it.
type Service struct {
	store *store.Store
	d     Deps

	mu     sync.Mutex
	synced map[string]time.Time // per-task last sync — the TTL memo
}

// New builds the comments service over the store and its hub seam.
func New(st *store.Store, d Deps) *Service {
	return &Service{store: st, d: d, synced: map[string]time.Time{}}
}

// ForView returns a task's comments for a detail read: the cached set, after a
// re-sync if it's stale. The sync is TTL-gated to once an hour and the detail read
// runs in a background command UI-side, so a view stays responsive. td-*/gh- only.
func (s *Service) ForView(project, id string) []store.Comment {
	if s.due(project, id) {
		if err := s.sync(project, id, false); err != nil {
			fmt.Fprintf(os.Stderr, "hub: sync comments for %s: %v\n", id, err)
		}
	}
	cs, err := s.store.For(project).Comments(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hub: read comments for %s: %v\n", id, err)
	}
	return cs
}

// LocalSource marks a comment this store owns, as against one mirrored from an issue tracker.
const LocalSource = "sindri"

// Add posts a comment on a task. Where the source keeps a thread of its own, the comment is written
// there and read straight back, so the issue's own readers see it and the canonical author, id and
// timestamp come from the source rather than being guessed here. Everywhere else this store is the
// thread, and the comment is simply recorded.
func (s *Service) Add(project, id, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("say something: an empty comment is not a comment")
	}
	if n, ok := github.Number(id); ok {
		ctx, cancel := context.WithTimeout(context.Background(), githubTimeout)
		defer cancel()
		if err := github.AddComment(ctx, s.d.ProjectRoot(project), n, body); err != nil {
			return err
		}
		// Forced, because the TTL would otherwise hide the comment just posted.
		if err := s.sync(project, id, true); err != nil {
			return err
		}
		s.d.Notify()
		return nil
	}
	ref, err := commentRef()
	if err != nil {
		return err
	}
	if err := s.store.For(project).AddComment(id, store.Comment{
		Source: LocalSource, SourceRef: ref, Author: "user", Body: body,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return err
	}
	s.d.Notify()
	return nil
}

// commentRef mints the per-source id the primary key needs. Random rather than a count, so two
// comments written in the same second cannot collide on it.
func commentRef() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate comment id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// Refresh forces a comment re-sync for one task (the [r]efresh key), bypassing the
// TTL, and notifies watchers.
func (s *Service) Refresh(project, id string) error {
	if err := s.sync(project, id, true); err != nil {
		return err
	}
	s.d.Notify()
	return nil
}

// sync fetches the task's comments from its source and reconciles the store against
// them (add/drop/update). A no-op when not due (unless forced) or for a source
// without comments (os-*). Records the sync time so the TTL holds.
func (s *Service) sync(project, id string, force bool) error {
	if !force && !s.due(project, id) {
		return nil
	}
	root := s.d.ProjectRoot(project)
	var (
		source   string
		comments []store.Comment
	)
	switch {
	case strings.HasPrefix(id, workflow.OwnedPrefix):
		// A task sindri owns has no upstream to reconcile against — the store IS the origin, so
		// syncing it would drop what is only held here.
		return nil
	case strings.HasPrefix(id, "gh-"):
		source = "github"
		n, ok := github.Number(id)
		if !ok {
			return fmt.Errorf("%s: not a valid GitHub id", id)
		}
		ctx, cancel := context.WithTimeout(context.Background(), githubTimeout)
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
	if err := s.store.For(project).ReplaceComments(id, source, comments); err != nil {
		return err
	}
	s.markSynced(project, id)
	return nil
}

// due reports whether a task's comments are stale (never synced, or older than ttl).
func (s *Service) due(project, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, ok := s.synced[project+"\x00"+id]
	return !ok || time.Since(last) >= ttl
}

// markSynced stamps a task's comments as freshly synced.
func (s *Service) markSynced(project, id string) {
	s.mu.Lock()
	s.synced[project+"\x00"+id] = time.Now()
	s.mu.Unlock()
}
