// package: github
// type:    adapter (external tool)
// job:     wraps the gh CLI so the hub can import a repo's open GitHub issues as a
// todo source and close+comment one when its local PR merges — reusing the
// user's existing gh auth, the same shell-out shape as the td/spec adapters.
// limits:  read issues + close-on-merge only; imports nothing from hub/store/issue,
// and never touches PRs (the local workflow owns those).
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/task"
)

// issueListLimit must be explicit: gh defaults to 30 and silently drops the rest.
const issueListLimit = 1000

// issueTimeout keeps a hung network call from stalling the source fetch.
const issueTimeout = 15 * time.Second

// ID is the stable task id for a GitHub issue: gh-<number>. Number reverses it.
func ID(number int) string { return "gh-" + strconv.Itoa(number) }

// Number reverses ID; ok=false for a non-gh id.
func Number(id string) (int, bool) {
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

// sourceTTL throttles this network source against the hub's frequent resyncs; force bypasses it.
const sourceTTL = 2 * time.Minute

// cacheEntry memoizes one repo's last good issue-tasks with the moment fetched.
type cacheEntry struct {
	tasks []task.Task
	at    time.Time
}

// cache is the per-repo memo owned by the source (the hub holds no GitHub state).
var (
	cacheMu sync.Mutex
	cache   = map[string]cacheEntry{}
)

// Source adapts GitHub issues as a task source. They import UNRATED, so no worker
// auto-claims an unvetted issue before a human rates it.
type Source struct{}

// Enabled adds the config opt-in to the package gate, so the hub needn't know of it.
func (Source) Enabled(root string) bool {
	if !Enabled(root) {
		return false
	}
	cfg, err := config.Load(root)
	return err == nil && cfg.IssuesEnabled()
}

// Tasks serves open issues from the TTL memo; a fetch error degrades to the last good list,
// so a network blip never fails the hub's sync.
func (Source) Tasks(root string, force bool) ([]task.Task, error) {
	cacheMu.Lock()
	entry, cached := cache[root]
	cacheMu.Unlock()
	if cached && !force && time.Since(entry.at) < sourceTTL {
		return entry.tasks, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), issueTimeout)
	defer cancel()
	issues, err := Issues(ctx, root)
	if err != nil {
		log.Printf("github source for %s degraded (keeping last list): %v", root, err)
		return entry.tasks, nil // last good (nil when never fetched) — never fail the sync
	}
	out := make([]task.Task, 0, len(issues))
	for _, is := range issues {
		out = append(out, task.Task{
			ID: ID(is.Number), Title: is.Title, Status: "open", Type: "issue",
			Priority: "", Description: is.Body, URL: is.URL,
		})
	}
	cacheMu.Lock()
	cache[root] = cacheEntry{tasks: out, at: time.Now()}
	cacheMu.Unlock()
	return out, nil
}

// OnMerged closes the issue behind a merged gh-* PR; best-effort, the local merge already landed.
func (Source) OnMerged(root, taskID, note string) error {
	number, ok := Number(taskID)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), issueTimeout)
	defer cancel()
	return Close(ctx, root, number, note)
}

// Finish closes (done) or hard-deletes (scrap, irreversible) a gh- issue.
func (Source) Finish(root, taskID string, scrap bool) (bool, error) {
	number, ok := Number(taskID)
	if !ok {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), issueTimeout)
	defer cancel()
	if scrap {
		return true, Delete(ctx, root, number)
	}
	return true, Close(ctx, root, number, "closed via sindri")
}

// Label is one GitHub label on an issue (only its name is used).
type Label struct {
	Name string `json:"name"`
}

// Issue mirrors `gh issue list --json`; the hub maps it, so this package stays task-model free.
type Issue struct {
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Labels    []Label `json:"labels"`
	UpdatedAt string  `json:"updatedAt"`
	URL       string  `json:"url"`
}

// Enabled is the cheap local gate (gh on PATH + a GitHub remote); it never probes network or auth,
// which is handled at call time.
func Enabled(root string) bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	return hasGitHubRemote(root)
}

// hasGitHubRemote looks for any github.com remote.
func hasGitHubRemote(root string) bool {
	cmd := exec.Command("git", "-C", root, "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "github.com")
}

// Issues lists open issues (gh already excludes PRs). Past Enabled(), a failure here is real —
// no auth, offline, rate-limited — and is surfaced for the caller to degrade on.
func Issues(ctx context.Context, root string) ([]Issue, error) {
	cmd := exec.CommandContext(ctx, "gh", "issue", "list",
		"--state", "open",
		"--limit", strconv.Itoa(issueListLimit),
		"--json", "number,title,body,labels,updatedAt,url",
	)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh issue list in %s: %w", root, ghError(err))
	}
	var issues []Issue
	if e := json.Unmarshal(out, &issues); e != nil {
		return nil, fmt.Errorf("parse gh issue list output: %w", e)
	}
	return issues, nil
}

// Comment is one issue comment; its URL is the stable sync key.
type Comment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	URL       string `json:"url"`
}

// IssueComments returns an issue's thread, oldest-first (GitHub's order).
func IssueComments(ctx context.Context, root string, number int) ([]Comment, error) {
	cmd := exec.CommandContext(ctx, "gh", "issue", "view", strconv.Itoa(number), "--json", "comments")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh issue view %d comments in %s: %w", number, root, ghError(err))
	}
	var resp struct {
		Comments []Comment `json:"comments"`
	}
	if e := json.Unmarshal(out, &resp); e != nil {
		return nil, fmt.Errorf("parse gh issue comments: %w", e)
	}
	return resp.Comments, nil
}

// Close closes an issue with a comment — the adapter's only outbound write.
func Close(ctx context.Context, root string, number int, comment string) error {
	cmd := exec.CommandContext(ctx, "gh", "issue", "close",
		strconv.Itoa(number), "--comment", comment)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh issue close %d in %s: %s", number, root, strings.TrimSpace(string(out)))
	}
	return nil
}

// Delete is GitHub's irreversible hard delete (needs admin/triage rights); it backs "scrap".
func Delete(ctx context.Context, root string, number int) error {
	cmd := exec.CommandContext(ctx, "gh", "issue", "delete", strconv.Itoa(number), "--yes")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh issue delete %d in %s: %s", number, root, strings.TrimSpace(string(out)))
	}
	return nil
}

// ghError surfaces gh's stderr instead of a bare "exit status 1".
func ghError(err error) error {
	if ee, ok := err.(*exec.ExitError); ok {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}
