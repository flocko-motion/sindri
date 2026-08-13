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
	e.referenceMoved(project, root, base, prev, tip, git.IsAncestor(root, prev, tip))
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
func (e *Engine) referenceMoved(project, root, base, prevTip, tip string, advanced bool) {
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
		// A branch under review, resolving, or gating must not move — a reviewer or a queued gate
		// is reading it right now, and a rebase would pull the ground out from under either.
		underReview := st.Phase == "submitted" || st.Phase == "resolving" || st.Phase == "gating"
		switch {
		case !advanced:
			_ = ps.Log(a.Name, "reference-rewritten", base)
			_ = e.deps.Deliver(project, a.Name, MsgReferenceRewritten(), MailAndPush)
		case underReview:
			// Leaving it unmoved is correct, but must leave a trace — otherwise the drift it lets
			// stand is unmeasurable afterwards.
			e.logReviewSkip(project, root, base, a)
			continue
		default:
			e.advanceAgent(project, root, base, prevTip, tip, a)
		}
	}
	e.deps.Notify()
}

// logReviewSkip records referenceMoved's one untraced decision: the STANDING drift against base,
// not this move's own delta — only that keeps meaning once it happens more than once.
func (e *Engine) logReviewSkip(project, root, base string, a store.Agent) {
	wt := filepath.Join(root, a.Workspace)
	behind, err := git.CountRange(wt, "HEAD", base)
	msg := base + ": under review, left unmoved"
	if err == nil {
		msg += fmt.Sprintf(" — %d commit(s) behind", behind)
	} else {
		msg += " — how far behind is unknown: " + err.Error()
	}
	_ = e.store.For(project).Log(a.Name, "reference-review-skip", msg)
}

// advanceAgent rebases one agent onto the advanced reference and reports it. A conflict is
// reported to the agent, not swallowed; a move that brought nothing stays silent, since a message
// that reliably says nothing teaches an agent to skim the channel.
func (e *Engine) advanceAgent(project, root, base, prevTip, tip string, a store.Agent) {
	ps := e.store.For(project)
	wt := filepath.Join(root, a.Workspace)
	// Measured against the tip the move was DECIDED FROM, not the branch name — re-resolving it
	// here would report a range nobody compared, and only before the rebase is it distinguishable
	// from the agent's own commits.
	arrived, countErr := git.CountRange(wt, prevTip, tip)
	incoming, _ := git.LogRange(wt, prevTip, tip, logCap)
	rebaseErr := git.Rebase(wt, base)

	// Silence only when it is KNOWN that nothing arrived: a failed count is not evidence of
	// nothing, and quiet there would leave an agent never hearing the reference moved at all.
	if countErr == nil && arrived == 0 {
		_ = ps.Log(a.Name, "reference-advanced-quiet", base+": moved, but nothing arrived here")
		return
	}
	if countErr != nil {
		_ = ps.Log(a.Name, "reference-count-failed", base+": "+countErr.Error())
	}
	if rebaseErr != nil {
		_ = ps.Log(a.Name, "reference-rebase-skip", base+": "+rebaseErr.Error())
		_ = e.deps.Deliver(project, a.Name, MsgReferenceNeedsRebase(incoming), MailAndPush)
		return
	}
	_ = ps.Log(a.Name, "reference-advanced", "rebased onto "+base)
	_ = e.deps.Deliver(project, a.Name, MsgReferenceAdvanced(incoming), PushOnly)
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
