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

// MaterializeReview checks branch out detached into the reserved .worktrees/review workspace, fresh
// each time, so a PR can be inspected without disturbing any agent's own worktree.
func MaterializeReview(root, branch string) (string, error) {
	path := filepath.Join(root, ".worktrees", "review")
	_ = git.WorktreeRemove(root, path) // fresh checkout each time
	if err := git.WorktreeAdd(root, path, branch); err != nil {
		return "", err
	}
	return path, nil
}

// gateName is the gate's own reserved worktree, not the review tree a human may be reading in.
const gateName = "gate"

// MaterializeGate checks ref out fresh and detached, so a gate measures the commit it is filed under
// rather than whatever the tree it came from holds by then.
func MaterializeGate(root, ref string) (string, error) {
	path := filepath.Join(root, ".worktrees", gateName)
	_ = git.WorktreeRemove(root, path)
	if err := git.WorktreeAdd(root, path, ref); err != nil {
		return "", err
	}
	return path, nil
}

// RemoveGate drops that worktree. Best-effort: tidying up must not turn a verdict into an error.
func RemoveGate(root string) {
	_ = git.WorktreeRemove(root, filepath.Join(root, ".worktrees", gateName))
}

// combinedName is the preflight's throwaway worktree and branch: one reserved name, so a crash
// leaves at most one stale tree rather than accumulating them.
const combinedName = "precheck"

// MaterializeCombined builds what a merge WOULD produce, in a throwaway worktree: its own branch at
// the PR tip, replayed onto base. Never the author's tree — replaying base where an agent is working
// moves the ground under it. The caller removes it either way (-> RemoveCombined).
func MaterializeCombined(root, branch, base string) (path string, conflicts []string, err error) {
	path = filepath.Join(root, ".worktrees", combinedName)
	RemoveCombined(root) // a previous run that died mid-flight leaves this behind
	if err := git.WorktreeAddOnBranch(root, path, combinedBranch(), branch); err != nil {
		return "", nil, err
	}
	conflicts, done, err := git.RebaseHere(path, base)
	if err != nil {
		return path, nil, err
	}
	if !done {
		return path, conflicts, nil
	}
	return path, nil, nil
}

// combinedBranch is the throwaway branch name. Prefixed so it is recognisable as the hub's own if
// one is ever left behind by a crash.
func combinedBranch() string { return "sindri-" + combinedName }

// RemoveCombined drops the throwaway worktree and its branch. Best-effort: it runs after the check
// has its answer, and failing here must not turn a finding into an error.
func RemoveCombined(root string) {
	_ = git.WorktreeRemove(root, filepath.Join(root, ".worktrees", combinedName))
	_ = git.DeleteBranch(root, combinedBranch())
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

// Gate checks wt in a subprocess, so the concurrent hub never chdir's. ONE of the two checks, never
// both: a declared verify (repo-relative, validated by config) owns the gate, since it is the thing
// that can run the built-in linter itself — running both paid for the same linter twice per gate.
func Gate(ctx context.Context, wt string, resolveBin func() (string, error), verify string) (output string, ok bool) {
	if verify != "" {
		return runVerify(ctx, wt, verify)
	}
	return builtinLint(wt, resolveBin)
}

// builtinLint is the gate brokkr provides, for a project that declares none of its own. A tree with
// no Go module passes: there is nothing here for it to say, and it is the only gate left.
func builtinLint(wt string, resolveBin func() (string, error)) (string, bool) {
	if _, err := os.Stat(filepath.Join(wt, "go.mod")); err != nil {
		return "", true
	}
	ok, out := lintgate.Adapter{ResolveBin: resolveBin}.Validate(wt)
	return out, ok
}

// runVerify executes the project's own declared command (not a tool sindri wraps, so no adapter
// applies), bounded and with its output capped. A timeout is a refusal, not a hang.
func runVerify(ctx context.Context, wt, verify string) (string, bool) {
	bin := filepath.Join(wt, filepath.FromSlash(verify))
	if _, err := os.Stat(bin); err != nil {
		return "verify: " + verify + " not found in the worktree — the project declares it in .sindri/config.yaml\n", false
	}
	ctx, cancel := context.WithTimeout(ctx, GateTimeout)
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
