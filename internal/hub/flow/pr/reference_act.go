// package: hub/flow/pr / reference_act
// type:    logic (the reference branch moving under live agents)
// job:     notice that a project's reference branch has moved, tell apart an ADVANCE
// from a REWRITE, and keep agents aligned — rebasing them onto an advance,
// warning them that a rewrite voided what they believed about the code.
// limits:  git mechanics live in adapter/git; the tick that calls this is the hub's.
package pr

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"path/filepath"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// refTipKey namespaces the remembered tip by project AND branch, so re-pointing `reference:` at a
// different branch reads as a fresh start rather than as that branch's history being rewritten.
func refTipKey(project, branch string) string { return "reftip:" + project + ":" + branch }

// SyncReference reacts to the project's reference branch having moved since the last look. The
// first look only records: with no previous tip nothing can be said to have moved.
func (a *Act) SyncReference(project string) error {
	root := a.Deps.ProjectRoot(project)
	if root == "" {
		return nil // repo not registered (or gone) — nothing to compare against
	}
	base, err := a.BaseBranch(root)
	if err != nil {
		return err
	}
	tip, err := git.BranchTip(root, base)
	if err != nil {
		return err
	}
	key := refTipKey(project, base)
	prev, seen, err := a.Store.GetMeta(key)
	if err != nil {
		return err
	}
	if err := a.Store.SetMeta(key, tip); err != nil {
		return err
	}
	if !seen || prev == tip {
		return nil
	}
	// Advanced = the old tip is still in the new history. Otherwise it was replaced, and every
	// agent's branch is forked from commits the reference no longer contains.
	a.referenceMoved(project, root, base, prev, tip, git.IsAncestor(root, prev, tip))
	return nil
}

// NoteReference records the reference's tip as already seen. Called after the hub itself moves it
// (a merge), so SyncReference doesn't re-report the hub's own work as an outside change.
func (a *Act) NoteReference(project string) {
	root := a.Deps.ProjectRoot(project)
	if root == "" {
		return
	}
	base, err := a.BaseBranch(root)
	if err != nil {
		return
	}
	if tip, err := git.BranchTip(root, base); err == nil {
		_ = a.Store.SetMeta(refTipKey(project, base), tip)
	}
}

// referenceMoved brings each agent into line with a moved reference. Best-effort per agent, as
// rebasePlanners is: one dirty worktree must not stop the rest being told.
func (a *Act) referenceMoved(project, root, base, prevTip, tip string, advanced bool) {
	ps := a.Store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	for _, ag := range roster {
		if ag.Workspace == "." {
			continue // a coauthor shares the user's own checkout; the hub never touches that
		}
		st, _ := ps.GetState(ag.Name)
		if st.Branch == "" {
			continue // nothing of its own in flight — it gets current material when it claims
		}
		// A branch under review, resolving, or gating must not move — a reviewer or a queued gate
		// is reading it right now, and a rebase would pull the ground out from under either.
		underReview := situation.Standing(st.Phase, "submitted", "resolving", "gating")
		switch {
		case !advanced:
			_ = ps.Log(ag.Name, "reference-rewritten", base)
			_ = a.Harness.Say(project, ag.Name, prompts.MsgReferenceRewritten(), mail.MailAndPush)
		case underReview:
			// Leaving it unmoved is correct, but must leave a trace — otherwise the drift it lets
			// stand is unmeasurable afterwards.
			a.logReviewSkip(project, root, base, ag)
			continue
		default:
			a.AdvanceAgent(project, root, base, prevTip, tip, ag)
		}
	}
	a.Deps.Notify()
}

// logReviewSkip records referenceMoved's one untraced decision: the STANDING drift against base,
// not this move's own delta — only that keeps meaning once it happens more than once.
func (a *Act) logReviewSkip(project, root, base string, ag store.Agent) {
	wt := filepath.Join(root, ag.Workspace)
	behind, err := git.CountRange(wt, "HEAD", base)
	msg := base + ": under review, left unmoved"
	if err == nil {
		msg += fmt.Sprintf(" — %d commit(s) behind", behind)
	} else {
		msg += " — how far behind is unknown: " + err.Error()
	}
	_ = a.Store.For(project).Log(ag.Name, "reference-review-skip", msg)
}

// AdvanceAgent rebases one agent onto the advanced reference and reports it. A conflict is
// reported to the agent, not swallowed; a move that brought nothing stays silent, since a message
// that reliably says nothing teaches an agent to skim the channel.
func (a *Act) AdvanceAgent(project, root, base, prevTip, tip string, ag store.Agent) {
	ps := a.Store.For(project)
	wt := filepath.Join(root, ag.Workspace)
	// Measured against the tip the move was DECIDED FROM, not the branch name — re-resolving it
	// here would report a range nobody compared, and only before the rebase is it distinguishable
	// from the agent's own commits.
	arrived, countErr := git.CountRange(wt, prevTip, tip)
	incoming, _ := git.LogRange(wt, prevTip, tip, core.LogCap)
	rebaseErr := git.Rebase(wt, base)

	// Silence only when it is KNOWN that nothing arrived: a failed count is not evidence of
	// nothing, and quiet there would leave an agent never hearing the reference moved at all.
	if countErr == nil && arrived == 0 {
		_ = ps.Log(ag.Name, "reference-advanced-quiet", base+": moved, but nothing arrived here")
		return
	}
	if countErr != nil {
		_ = ps.Log(ag.Name, "reference-count-failed", base+": "+countErr.Error())
	}
	if rebaseErr != nil {
		_ = ps.Log(ag.Name, "reference-rebase-skip", base+": "+rebaseErr.Error())
		_ = a.Harness.Say(project, ag.Name, prompts.MsgReferenceNeedsRebase(incoming), mail.MailAndPush)
		return
	}
	_ = ps.Log(ag.Name, "reference-advanced", "rebased onto "+base)
	_ = a.Harness.Say(project, ag.Name, prompts.MsgReferenceAdvanced(incoming), mail.PushOnly)
}
