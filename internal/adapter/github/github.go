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
)

// issueListLimit is passed to `gh issue list` explicitly: gh defaults to 30 and
// would silently drop the rest, so we ask for a high ceiling to import them all.
const issueListLimit = 1000

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
