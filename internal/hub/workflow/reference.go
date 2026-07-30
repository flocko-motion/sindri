// package: hub/workflow / reference
// type:    logic (the reference branch moving under live agents)
// job:     notice that a project's reference branch has moved, tell apart an ADVANCE
// from a REWRITE, and keep agents aligned — rebasing them onto an advance,
// warning them that a rewrite voided what they believed about the code.
// limits:  git mechanics live in adapter/git; the tick that calls this is the hub's.
package workflow

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/store"
)

// refTipKey namespaces the remembered tip by project AND branch, so re-pointing `reference:` at a
// different branch reads as a fresh start rather than as that branch's history being rewritten.
func refTipKey(project, branch string) string { return "reftip:" + project + ":" + branch }

// SyncReference reacts to the project's reference branch having moved since the last look. The
// first look only records: with no previous tip nothing can be said to have moved.
func (e *Engine) SyncReference(project string) error {
	root := e.deps.ProjectRoot(project)
	if root == "" {
		return nil // repo not registered (or gone) — nothing to compare against
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return err
	}
	tip, err := git.BranchTip(root, base)
	if err != nil {
		return err
	}
	key := refTipKey(project, base)
	prev, seen, err := e.store.GetMeta(key)
	if err != nil {
		return err
	}
	if err := e.store.SetMeta(key, tip); err != nil {
		return err
	}
	if !seen || prev == tip {
		return nil
	}
	// Advanced = the old tip is still in the new history. Otherwise it was replaced, and every
	// agent's branch is forked from commits the reference no longer contains.
	e.referenceMoved(project, root, base, prev, git.IsAncestor(root, prev, tip))
	return nil
}

// noteReference records the reference's tip as already seen. Called after the hub itself moves it
// (a merge), so SyncReference doesn't re-report the hub's own work as an outside change.
func (e *Engine) noteReference(project string) {
	root := e.deps.ProjectRoot(project)
	if root == "" {
		return
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return
	}
	if tip, err := git.BranchTip(root, base); err == nil {
		_ = e.store.SetMeta(refTipKey(project, base), tip)
	}
}

// referenceMoved brings each agent into line with a moved reference. Best-effort per agent, as
// rebasePlanners is: one dirty worktree must not stop the rest being told.
func (e *Engine) referenceMoved(project, root, base, prevTip string, advanced bool) {
	ps := e.store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	for _, a := range roster {
		if a.Workspace == "." {
			continue // a coauthor shares the user's own checkout; the hub never touches that
		}
		st, _ := ps.GetState(a.Name)
		if st.Branch == "" {
			continue // nothing of its own in flight — it gets current material when it claims
		}
		// A branch under review must not move: the reviewer is reading the diff that was
		// submitted. A rewrite is still worth saying, since it can invalidate that diff's base.
		underReview := st.Phase == "submitted" || st.Phase == "resolving"
		switch {
		case !advanced:
			_ = ps.Log(a.Name, "reference-rewritten", base)
			_ = e.deps.InjectWhenReady(project, a.Name, MsgReferenceRewritten())
		case underReview:
			continue // the merge rebases it when the time comes; saying so now is noise
		default:
			e.advanceAgent(project, root, base, prevTip, a)
		}
	}
	e.deps.Notify()
}

// advanceAgent rebases one agent onto the advanced reference and tells it what arrived. A rebase it
// cannot do cleanly is reported to the agent, not swallowed — it owns the conflict.
func (e *Engine) advanceAgent(project, root, base, prevTip string, a store.Agent) {
	ps := e.store.For(project)
	wt := filepath.Join(root, a.Workspace)
	// Read what is arriving BEFORE the rebase: afterwards those commits are indistinguishable
	// from the agent's own history.
	incoming, _ := git.LogRange(wt, prevTip, base, logCap)
	if err := git.Rebase(wt, base); err != nil {
		_ = ps.Log(a.Name, "reference-rebase-skip", base+": "+err.Error())
		_ = e.deps.InjectWhenReady(project, a.Name, MsgReferenceNeedsRebase(incoming))
		return
	}
	_ = ps.Log(a.Name, "reference-advanced", "rebased onto "+base)
	_ = e.deps.InjectWhenReady(project, a.Name, MsgReferenceAdvanced(incoming))
}

// commitList renders incoming commits as an indented block, or "" when there are none to name.
func commitList(incoming []string) string {
	if len(incoming) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nWhat arrived (%d):\n", len(incoming))
	for _, l := range incoming {
		b.WriteString("  " + l + "\n")
	}
	return b.String()
}
