// package: github
// type:    adapter (external tool)
// job:     wraps the gh CLI so the hub can import a repo's open GitHub issues as a
//          todo source and close+comment one when its local PR merges — reusing the
//          user's existing gh auth, the same shell-out shape as the td/spec adapters.
// limits:  read issues + close-on-merge only; imports nothing from hub/store/issue,
//          and never touches PRs (the local workflow owns those).
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/hub/task"
)

// issueListLimit is passed to `gh issue list` explicitly: gh defaults to 30 and
// would silently drop the rest, so we ask for a high ceiling to import them all.
const issueListLimit = 1000

// issueTimeout bounds a single `gh issue list` so a hung network call can't stall
// the source fetch.
const issueTimeout = 15 * time.Second

// ID is the stable task id for a GitHub issue: gh-<number>. Number reverses it.
func ID(number int) string { return "gh-" + strconv.Itoa(number) }

// Number parses a gh-<number> task id back to its issue number (ok=false for a
// non-gh id).
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

// Source adapts GitHub issues as a task source. Issues import UNRATED (empty
// priority) so a worker never auto-claims an unvetted issue until a human rates it.
type Source struct{}

// Enabled reports whether the repo can use the GitHub source (gh on PATH + a GitHub
// remote). The per-project opt-in is the hub's concern, layered on top.
func (Source) Enabled(root string) bool { return Enabled(root) }

// Tasks fetches the repo's open issues as domain tasks (gh-* ids, the body as the
// description), bounded by its own timeout.
func (Source) Tasks(root string) ([]task.Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), issueTimeout)
	defer cancel()
	issues, err := Issues(ctx, root)
	if err != nil {
		return nil, err
	}
	out := make([]task.Task, 0, len(issues))
	for _, is := range issues {
		out = append(out, task.Task{
			ID: ID(is.Number), Title: is.Title, Status: "open", Type: "issue",
			Priority: "", Description: is.Body,
		})
	}
	return out, nil
}

// Label is one GitHub label on an issue (only its name is used).
type Label struct {
	Name string `json:"name"`
}

// Issue is an open GitHub issue as returned by `gh issue list --json`. It is the
// adapter's own type — the hub maps it to store.Task, keeping this package
// ignorant of the task model.
type Issue struct {
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Labels    []Label `json:"labels"`
	UpdatedAt string  `json:"updatedAt"`
}

// Enabled reports whether the GitHub source can even be attempted: the gh CLI is
// on PATH and the repo has a GitHub remote. It is cheap, local, and
// side-effect-free — it does NOT probe the network or check auth. An
// unauthenticated/offline gh is handled at call time (Issues returns an error and
// the caller degrades to no tasks).
func Enabled(root string) bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	return hasGitHubRemote(root)
}

// hasGitHubRemote reports whether the repo at root has any remote pointing at
// github.com — the local signal that this is a GitHub-backed repo.
func hasGitHubRemote(root string) bool {
	cmd := exec.Command("git", "-C", root, "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "github.com")
}

// Issues lists the repo's open issues via
// `gh issue list --state open --limit <high> --json number,title,body,labels,updatedAt`.
// gh issue list already excludes pull requests. The explicit high --limit is
// required — gh defaults to 30. After Enabled() is true, a failure here is a real
// error (gh missing auth / offline / rate-limited), surfaced so the caller can
// degrade to contributing no tasks this cycle.
func Issues(ctx context.Context, root string) ([]Issue, error) {
	cmd := exec.CommandContext(ctx, "gh", "issue", "list",
		"--state", "open",
		"--limit", strconv.Itoa(issueListLimit),
		"--json", "number,title,body,labels,updatedAt",
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

// Comment is one comment on a GitHub issue. URL is its stable, unique reference
// (used as the sync key); Author is the commenter's login.
type Comment struct {
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	URL       string `json:"url"`
}

// IssueComments returns an issue's comment thread via
// `gh issue view <n> --json comments`. Ordered oldest-first (GitHub's order).
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

// Close closes issue number with a comment via
// `gh issue close <number> --comment <comment>` — the ONLY outbound write this
// adapter makes, used by the hub's close-on-merge path (best-effort there).
func Close(ctx context.Context, root string, number int, comment string) error {
	cmd := exec.CommandContext(ctx, "gh", "issue", "close",
		strconv.Itoa(number), "--comment", comment)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh issue close %d in %s: %s", number, root, strings.TrimSpace(string(out)))
	}
	return nil
}

// Delete permanently deletes issue number via `gh issue delete <number> --yes`. This
// is GitHub's hard delete (irreversible, and requires repo-admin/triage rights); it
// backs the "scrap" close for a gh-* task, as opposed to Close's "done".
func Delete(ctx context.Context, root string, number int) error {
	cmd := exec.CommandContext(ctx, "gh", "issue", "delete", strconv.Itoa(number), "--yes")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh issue delete %d in %s: %s", number, root, strings.TrimSpace(string(out)))
	}
	return nil
}

// ghError attaches gh's stderr (carried on ExitError) to the error, so an auth or
// network failure surfaces gh's own message rather than a bare "exit status 1".
func ghError(err error) error {
	if ee, ok := err.(*exec.ExitError); ok {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}
