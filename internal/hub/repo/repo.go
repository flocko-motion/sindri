// package: hub/repo / repo
// type:    logic (git/PR mechanics)
// job:     the git-backed operations the workflow orchestrates — materialize a PR
// branch for inspection, run the submit gate against a worktree. Stateless:
// each takes explicit paths/refs and returns a result or error; the workflow
// resolves PR records and decides consequences.
// limits:  no store, no orchestration, no agent messaging. git primitives live in
// adapter/git; this drives them for the PR lifecycle.
package repo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/lintgate"
)

// MaterializeReview checks out branch (detached) into the repo's reserved
// .worktrees/review workspace — fresh each time — and returns the path, so a human or
// reviewer can inspect a PR branch without disturbing any agent's own worktree.
func MaterializeReview(root, branch string) (string, error) {
	path := filepath.Join(root, ".worktrees", "review")
	_ = git.WorktreeRemove(root, path) // fresh checkout each time
	if err := git.WorktreeAdd(root, path, branch); err != nil {
		return "", err
	}
	return path, nil
}

// ScrapBranch removes a discarded PR's branch, detaching the owning worktree first since git will
// not delete a checked-out one. The detach is best-effort; the delete's outcome is returned.
func ScrapBranch(root, worktree, branch string) error {
	if worktree != "" {
		if _, err := os.Stat(worktree); err == nil {
			_ = git.DetachHead(worktree)
		}
	}
	return git.DeleteBranch(root, branch)
}

// GateTimeout bounds the project's own verify command. Generous, because a real gate builds and
// runs a test suite; bounded, because an agent waiting forever on a hung gate reports nothing at all.
const GateTimeout = 15 * time.Minute

// gateOutputLines caps stored gate output, the way the diff commands cap theirs.
const gateOutputLines = 400

// Gate runs the submit gate in a worktree as a subprocess, so the concurrent hub never chdir's: the
// built-in lint, then the project's own verify command when it declares one. Named for what it now
// is — a gate — since it may build and test, not only lint.
//
// verify is repo-relative and already validated by config; "" means the project declares none, and
// then the built-in behaviour is exactly what it was, including the silent pass for a non-Go tree.
// A declared gate runs whatever the language, because the project asked for it.
func Gate(wt string, resolveBin func() (string, error), verify string) (output string, ok bool) {
	if out, passed := builtinLint(wt, resolveBin, verify != ""); !passed {
		return out, false
	} else if verify == "" {
		return out, true
	}
	return runVerify(wt, verify)
}

// builtinLint is the gate brokkr provides. skipGoCheck keeps a non-Go tree in play when the project
// has declared its own gate — otherwise a project in another language would still gate on nothing.
func builtinLint(wt string, resolveBin func() (string, error), declared bool) (string, bool) {
	if _, err := os.Stat(filepath.Join(wt, "go.mod")); err != nil {
		if declared {
			return "", true // not a Go tree: nothing for the built-in to say, the declared gate decides
		}
		return "", true // no Go module and no declared gate — as before
	}
	ok, out := lintgate.Adapter{ResolveBin: resolveBin}.Validate(wt)
	return out, ok
}

// runVerify executes the project's own gate, bounded and with its output capped. A timeout is a
// refusal, not a hang: the agent is told the gate ran out of time and how long it had. Its
// exec.CommandContext runs a path the PROJECT declares, not a tool sindri depends on and wraps
// (git, brokkr, ...) — no adapter applies to running a caller-supplied command; that's the feature.
func runVerify(wt, verify string) (string, bool) {
	bin := filepath.Join(wt, filepath.FromSlash(verify))
	if _, err := os.Stat(bin); err != nil {
		return "verify: " + verify + " not found in the worktree — the project declares it in .sindri/config.yaml\n", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), GateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = wt
	out, err := cmd.CombinedOutput()
	body := capLines(string(out), gateOutputLines)
	if ctx.Err() != nil {
		return body + fmt.Sprintf("verify: %s ran past %s and was stopped — the gate refuses rather than waiting.\n", verify, GateTimeout), false
	}
	if err != nil {
		return body + "verify: " + verify + " failed (" + err.Error() + ")\n", false
	}
	return body, true
}

// capLines truncates long output and says what was cut, because silent truncation reads as a
// complete answer — the same reason the diff commands cap theirs.
func capLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		if len(s) == 0 {
			return ""
		}
		return strings.Join(lines, "\n") + "\n"
	}
	return fmt.Sprintf("%s\n… truncated: last %d of %d lines shown — re-run the gate locally for the rest.\n",
		strings.Join(lines[len(lines)-max:], "\n"), max, len(lines))
}
