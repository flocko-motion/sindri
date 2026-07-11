package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGH writes a stub `gh` onto PATH that answers `issue list` from GH_ISSUES_FILE,
// records `issue close` invocations to GH_CLOSE_LOG, and fails (exit 1, stderr) when
// GH_FAIL is set — so tests drive the adapter without a real GitHub. Real `git` stays
// resolvable (the fake dir is prepended, not a replacement).
func fakeGH(t *testing.T) (closeLog string) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
if [ -n "$GH_FAIL" ]; then echo "gh: authentication required" 1>&2; exit 1; fi
if [ "$1" = "issue" ] && [ "$2" = "list" ]; then cat "$GH_ISSUES_FILE"; exit 0; fi
if [ "$1" = "issue" ] && [ "$2" = "close" ]; then echo "$@" >> "$GH_CLOSE_LOG"; exit 0; fi
exit 1
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	closeLog = filepath.Join(t.TempDir(), "close.log")
	t.Setenv("GH_CLOSE_LOG", closeLog)
	return closeLog
}

// gitHubRepo makes a real git repo with a github.com remote, so Enabled's local
// remote check passes.
func gitHubRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "origin", "https://github.com/acme/widgets.git"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	return root
}

func writeIssues(t *testing.T, issues []Issue) {
	t.Helper()
	data, err := json.Marshal(issues)
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "issues.json")
	if err := os.WriteFile(f, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_ISSUES_FILE", f)
}

func TestEnabled(t *testing.T) {
	fakeGH(t)

	// gh present + github remote → enabled.
	if !Enabled(gitHubRepo(t)) {
		t.Error("expected enabled with gh on PATH and a github remote")
	}

	// A repo with no github remote → disabled (even with gh present).
	plain := t.TempDir()
	if out, err := exec.Command("git", "-C", plain, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s", out)
	}
	if Enabled(plain) {
		t.Error("a repo without a github remote must not be enabled")
	}

	// gh missing → disabled regardless of remote. (Build the repo first — hiding
	// PATH also hides the real git that gitHubRepo needs.)
	ghRepo := gitHubRepo(t)
	t.Setenv("PATH", "")
	if Enabled(ghRepo) {
		t.Error("must be disabled when gh is not on PATH")
	}
}

func TestIssuesParses(t *testing.T) {
	fakeGH(t)
	root := gitHubRepo(t)
	writeIssues(t, []Issue{
		{Number: 12, Title: "Fix the thing", Body: "details", Labels: []Label{{Name: "bug"}}, UpdatedAt: "2026-01-01T00:00:00Z"},
		{Number: 7, Title: "Add a feature"},
	})

	got, err := Issues(context.Background(), root)
	if err != nil {
		t.Fatalf("Issues: %v", err)
	}
	if len(got) != 2 || got[0].Number != 12 || got[0].Title != "Fix the thing" || got[0].Labels[0].Name != "bug" {
		t.Fatalf("unexpected parse: %+v", got)
	}
}

// TestIssuesReturnsAllOverThirty guards the explicit --limit: gh defaults to 30, so a
// 50-issue repo must still return all 50. (The stub echoes whatever we hand it; the
// real guarantee is the --limit flag the adapter passes, asserted below.)
func TestIssuesReturnsAllOverThirty(t *testing.T) {
	fakeGH(t)
	root := gitHubRepo(t)
	var many []Issue
	for i := 1; i <= 50; i++ {
		many = append(many, Issue{Number: i, Title: fmt.Sprintf("issue %d", i)})
	}
	writeIssues(t, many)

	got, err := Issues(context.Background(), root)
	if err != nil {
		t.Fatalf("Issues: %v", err)
	}
	if len(got) != 50 {
		t.Fatalf("expected all 50 issues, got %d", len(got))
	}
	if issueListLimit <= 30 {
		t.Fatalf("issueListLimit must exceed gh's default of 30, is %d", issueListLimit)
	}
}

func TestIssuesSurfacesError(t *testing.T) {
	fakeGH(t)
	root := gitHubRepo(t)
	writeIssues(t, nil)
	t.Setenv("GH_FAIL", "1")

	if _, err := Issues(context.Background(), root); err == nil {
		t.Fatal("an authentication/offline failure must surface as an error, not empty success")
	} else if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("error should carry gh's stderr, got: %v", err)
	}
}

func TestCloseCommandShape(t *testing.T) {
	closeLog := fakeGH(t)
	root := gitHubRepo(t)

	if err := Close(context.Background(), root, 42, "merged by sindri: td-1/pr-1"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err := os.ReadFile(closeLog)
	if err != nil {
		t.Fatalf("read close log: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if !strings.Contains(got, "issue close 42") || !strings.Contains(got, "--comment") || !strings.Contains(got, "merged by sindri") {
		t.Errorf("unexpected close invocation: %q", got)
	}
}

func TestCloseSurfacesError(t *testing.T) {
	fakeGH(t)
	root := gitHubRepo(t)
	t.Setenv("GH_FAIL", "1")

	if err := Close(context.Background(), root, 42, "x"); err == nil {
		t.Fatal("a failed close must return an error")
	}
}
