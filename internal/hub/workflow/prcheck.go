// package: hub/workflow / prcheck
// type:    logic (keeping an open PR honest as its base moves)
// job:     after the reference branch moves, answer for each open PR whether it would still
// apply and whether the combined result would pass the project gate — recording the
// finding on the PR for the human rather than acting on it.
// limits:  git mechanics are adapter/git's and the throwaway worktree repo's; the cadence
// that calls this is refwatch's.
package workflow

import (
	"fmt"
	"strings"
	"sync"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// prCheckPaths caps how many conflicting paths a finding names. A verdict with no evidence sends
// the author looking; the whole list of a wide conflict is not evidence, it is noise.
const prCheckPaths = 10

// preflight serialises the checks and remembers what has already been answered. ONE AT A TIME, and
// one PR per sweep: tier 2 may build and test, so several open PRs on a moving base would otherwise
// leave the host permanently busy.
type preflight struct {
	mu   sync.Mutex
	seen map[string]string // PR id -> the base+branch tips already checked
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
	base, err := e.baseBranch(root)
	if err != nil {
		return
	}
	baseTip, err := git.BranchTip(root, base)
	if err != nil {
		return
	}
	ps := e.store.For(project)
	prs, err := ps.PRs()
	if err != nil {
		return
	}
	for _, pr := range prs {
		if !e.preflightWanted(pr, root, base, baseTip) {
			continue
		}
		e.preflightPR(project, ps, pr, root, base, baseTip)
		return // one per sweep: the next moves on the next tick
	}
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
	if e.pre.seen[pr.ID] == baseTip+":"+branchTip {
		return false // already answered at these tips; a burst of merges re-answers once, not per merge
	}
	return true
}

// preflightPR is tier 2: materialise what would land and gate it. Its rebase is the definitive
// applies-answer — the same replay the merge performs, in a tree nobody is working in.
func (e *Engine) preflightPR(project string, ps *store.ProjectStore, pr store.PR, root, base, baseTip string) {
	branchTip, err := git.BranchTip(root, pr.Branch)
	if err != nil {
		return
	}
	e.pre.seen[pr.ID] = baseTip + ":" + branchTip

	path, conflicts, err := repo.MaterializeCombined(root, pr.Branch, base)
	defer repo.RemoveCombined(root)
	if err != nil {
		// The check itself failed — say so as a check failure, never as a finding about the PR.
		_ = ps.LogPR(pr.ID, "precheck-skipped", trimTo(err.Error(), 200))
		return
	}
	if len(conflicts) > 0 {
		_ = ps.LogPR(pr.ID, "precheck-conflict", conflictNote(base, conflicts))
		e.deps.Notify()
		return
	}
	out, passed := repo.Gate(path, e.deps.BrokkrBin, e.verifyCmd(project))
	if passed {
		_ = ps.LogPR(pr.ID, "precheck-pass", "applies onto "+base+" and the gate passes on the combined result")
		e.deps.Notify()
		return
	}
	// Quote the failure. A bare "the gate fails" makes the author reproduce the whole run to find
	// out what it was, and the combined tree it failed in no longer exists by then.
	_ = ps.LogPR(pr.ID, "precheck-gate-fail", "combined with "+base+" the gate fails:\n"+trimTo(out, 1200))
	e.deps.Notify()
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
