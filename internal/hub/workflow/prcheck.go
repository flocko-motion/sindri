// package: hub/workflow / prcheck
// type:    logic (keeping an open PR honest as its base moves)
// job:     after the reference branch moves, answer for each open PR whether it would still
// apply and whether the combined result would pass the project gate — recording the
// finding on the PR for the human rather than acting on it.
// limits:  git mechanics are adapter/git's and the throwaway worktree repo's; the cadence
// that calls this is refwatch's.
package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// prCheckPaths caps how many conflicting paths a finding names. A verdict with no evidence sends
// the author looking; the whole list of a wide conflict is not evidence, it is noise.
const prCheckPaths = 10

// preflight serialises the DECIDING and remembers what has already been answered — one PR per sweep,
// so a moving base cannot queue a check per open PR at once. What keeps two checks from building at
// the same time is the run queue they now go through, not this mutex (-> executePrecheckRun).
type preflight struct {
	mu   sync.Mutex
	seen map[string]string // PR id -> the base and tips already checked (-> prCheckKey)
}

// CheckOpenPRs answers, per open PR, whether it still applies and whether the combined result would
// pass the gate. ADVISORY: it records the finding and stops. Rejecting would route the PR back to
// its author mid-review and, on a busy base, invite move → reject → rebase → resubmit → move.
func (e *Engine) CheckOpenPRs(project string) {
	if !e.pre.mu.TryLock() {
		return // a check is already running; this sweep has nothing to add
	}
	defer e.pre.mu.Unlock()

	root := e.deps.ProjectRoot(project)
	if root == "" {
		return
	}
	// The project reference is only the FALLBACK, for PR rows old enough to carry no base of their
	// own. Its failure is not fatal here: a PR that names its own base needs nothing from it.
	fallback, _ := e.baseBranch(root)
	ps := e.store.For(project)
	prs, err := ps.PRs()
	if err != nil {
		return
	}
	for _, pr := range prs {
		base, baseTip, ok := e.prBase(root, pr, fallback)
		if !ok || !e.preflightWanted(pr, root, base, baseTip) {
			continue
		}
		e.preflightPR(project, ps, pr, root, base, baseTip)
		return // one per sweep: the next moves on the next tick
	}
}

// prBase is the base THIS PR will be merged onto, and its tip. Per PR, never project-wide: the merge
// replays onto pr.Base (-> repo.MergeBranch) and names it in every conflict, so a check that used
// the current project reference would answer about an operation nobody will perform — and could
// report clean a PR that conflicts with its real base. Re-pointing `reference:` is exactly the event
// this check runs on, so the two diverge precisely when it matters.
func (e *Engine) prBase(root string, pr store.PR, fallback string) (base, tip string, ok bool) {
	base = pr.Base
	if base == "" {
		base = fallback // an older row, recorded before a PR carried its own base
	}
	if base == "" {
		return "", "", false
	}
	tip, err := git.BranchTip(root, base)
	if err != nil {
		return "", "", false // the base is gone; nothing to check against
	}
	return base, tip, true
}

// preflightWanted is tier 1: the cheap filter, no checkout, nothing disturbed. It asks only whether
// the base moved past this PR — a check modelling a MERGE would answer a different question from the
// rebase the hub performs, so the cheap tier decides whether to LOOK and tier 2 gives the verdict.
func (e *Engine) preflightWanted(pr store.PR, root, base, baseTip string) bool {
	if !api.PROpen(pr) || pr.Branch == "" || pr.Branch == base {
		return false
	}
	branchTip, err := git.BranchTip(root, pr.Branch)
	if err != nil {
		return false // the branch is gone or unreadable; nothing to say about it
	}
	behind, err := git.CountRange(root, pr.Branch, base)
	if err != nil || behind == 0 {
		// A failed count is not evidence of "up to date", but it is no basis for a finding either.
		return false
	}
	if e.pre.seen[pr.ID] == prCheckKey(base, baseTip, branchTip) {
		return false // already answered at these tips; a burst of merges re-answers once, not per merge
	}
	return true
}

// preflightPR is tier 2: QUEUE the materialise-and-gate, rather than run it here. The check builds
// and tests, so it belongs in the fleet's one slot with every other gate — its own mutex kept two
// prechecks apart but nothing kept one out of a submitting agent's way.
func (e *Engine) preflightPR(project string, ps *store.ProjectStore, pr store.PR, root, base, baseTip string) {
	branchTip, err := git.BranchTip(root, pr.Branch)
	if err != nil {
		return
	}
	// Marked as answered when the work is QUEUED, not when it lands: the sweep runs every 30s and a
	// gate takes minutes, so re-asking while the run waits would queue the same check many times.
	e.pre.seen[pr.ID] = prCheckKey(base, baseTip, branchTip)
	if e.queuedPrecheck(ps, pr.ID) {
		return // one is already waiting, and it will answer about the tips it finds when it runs
	}
	if _, err := e.putRun(project, store.Run{
		Agent: api.SenderSystem, Kind: gatePrecheck, Message: pr.ID,
		Command: "gate: precheck " + pr.ID + " onto " + base,
	}); err != nil {
		_ = ps.LogPR(pr.ID, "precheck-skipped", trimTo(err.Error(), 200))
	}
}

// executePrecheckRun is that check, from the queue: build what a merge would produce and gate it.
// The rebase is the definitive applies-answer — the same replay the merge performs, in a tree
// nobody is working in. ADVISORY throughout: the finding is logged on the PR and nobody is told.
func (e *Engine) executePrecheckRun(ctx context.Context, ps *store.ProjectStore, project string, r api.Run) error {
	pr, ok, err := ps.GetPR(r.Message)
	if err != nil || !ok {
		return e.finishRun(ps, project, r, "cancelled", "precheck: "+r.Message+" is gone\n", 0, 0, -1)
	}
	root := e.deps.ProjectRoot(project)
	fallback, _ := e.baseBranch(root)
	base, _, resolved := e.prBase(root, pr, fallback)
	if !resolved {
		return e.finishRun(ps, project, r, "cancelled", "precheck: the base of "+pr.ID+" is gone\n", 0, 0, -1)
	}
	if err := ps.SetRunStatus(r.ID, "running"); err != nil {
		return err
	}
	e.deps.Notify()
	start := time.Now()
	path, conflicts, err := repo.MaterializeCombined(root, pr.Branch, base)
	defer repo.RemoveCombined(root)
	if err != nil {
		// The check itself failed — say so as a check failure, never as a finding about the PR.
		_ = ps.LogPR(pr.ID, "precheck-skipped", trimTo(err.Error(), 200))
		return e.finishRun(ps, project, r, "cancelled", "precheck: "+err.Error()+"\n", time.Since(start), RunHardCap, -1)
	}
	if len(conflicts) > 0 {
		_ = ps.LogPR(pr.ID, "precheck-conflict", conflictNote(base, conflicts))
		return e.finishRun(ps, project, r, "failed", conflictNote(base, conflicts)+"\n", time.Since(start), RunHardCap, 1)
	}
	sha, err := git.Head(path)
	if err != nil {
		return e.finishRun(ps, project, r, "cancelled", "precheck: "+err.Error()+"\n", time.Since(start), RunHardCap, -1)
	}
	// Not recorded against the commit: the combined replay is thrown away with its worktree, so a
	// verdict about that sha could never be reused (-> gateOnce).
	out, passed := e.gateOnce(ctx, project, path, sha)
	if passed {
		_ = ps.LogPR(pr.ID, "precheck-pass", "applies onto "+base+" and the gate passes on the combined result")
		return e.finishRun(ps, project, r, "passed", out, time.Since(start), RunHardCap, 0)
	}
	// Quote the failure. A bare "the gate fails" makes the author reproduce the whole run to find
	// out what it was, and the combined tree it failed in no longer exists by then.
	_ = ps.LogPR(pr.ID, "precheck-gate-fail", "combined with "+base+" the gate fails:\n"+trimTo(out, 1200))
	return e.finishRun(ps, project, r, "failed", out, time.Since(start), RunHardCap, 1)
}

// prCheckKey is what a PR has already been answered at. The base NAME is in it, not just its tip:
// re-pointing a PR at a different branch is a new question even when both tips are unchanged.
func prCheckKey(base, baseTip, branchTip string) string {
	return base + ":" + baseTip + ":" + branchTip
}

// conflictNote names the paths, which is what makes the finding actionable — a verdict without them
// sends the author hunting through a diff for something the hub already knew.
func conflictNote(base string, conflicts []string) string {
	shown := conflicts
	extra := ""
	if len(shown) > prCheckPaths {
		extra = fmt.Sprintf(" (+%d more)", len(shown)-prCheckPaths)
		shown = shown[:prCheckPaths]
	}
	return "no longer applies onto " + base + " — conflicts in " + strings.Join(shown, ", ") + extra
}

// trimTo caps a stored payload, keeping the TAIL of gate output: a failing run says what failed at
// the end, and the head is the part that passed.
func trimTo(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-max:]
}
