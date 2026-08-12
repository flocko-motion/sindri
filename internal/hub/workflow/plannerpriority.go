// package: hub/workflow / planner priority
// type:    logic (the planner's ordering verb)
// job:     let a planner express sequence — rate its proposals and re-order rated work — while
// refusing any rating that would change whether a task is claimable.
// limits:  the priority field only; approval stays entirely the user's, here and everywhere.
package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
)

// authorisedForClaim reports whether the approval gate would let this task out. It mirrors the claim
// queries' `status IS NULL OR status='approved'` (-> store/tasks.go), so a task with NO approval row
// counts as authorised — that is the ordinary state of a task nobody proposed.
func authorisedForClaim(approval string) bool {
	return approval == "" || approval == "approved"
}

// plannerMayRate is THE INVARIANT: no planner action changes whether a task is claimable. Claimable
// is a priority AND authorisation, so a planner may move a priority between values, and may rate
// anything the gate still holds — but on an authorised task it may not cross between unrated and
// rated, because that one act releases work, or withdraws work already released.
func plannerMayRate(approval, current, next string) (ok bool, why string) {
	if !authorisedForClaim(approval) {
		return true, "" // unclaimable either way; a rating here only proposes an order
	}
	switch {
	case current == "" && next != "":
		return false, "it is approved and unrated, so setting a priority would release it to a worker"
	case current != "" && next == "":
		return false, "it is approved and rated, so clearing its priority would withdraw work already released"
	}
	return true, "" // rated to rated: the order changes, the authorisation does not
}

// prioritiseUsage is the one description of the verb's surface.
const prioritiseUsage = "usage: prioritise-task <id> <critical|high|mid|low|none>\n" +
	"  Order the work you planned: rate a proposal, or re-sequence tasks that already carry a rating.\n" +
	"  You cannot set a first priority on an approved task, or clear one — either would change\n" +
	"  whether a worker can take it, and releasing work is the user's decision."

// PrioritiseTaskHelp is what the command registry advertises.
const PrioritiseTaskHelp = "set the order of work you planned. " + prioritiseUsage

// CmdPrioritiseTask is the planner's ordering verb. Separate from edit-task on purpose: edit-task is
// pending-only because approval means the user took the task AS IT STANDS, and that is about
// content. Ordering is not content — re-sequencing live backlog is the job — so folding it in would
// make edit-task's rule depend on which flags were passed and weaken it for the fields it protects.
//
// One task at a time, no scope: a cascade would have to test the boundary per target, and one
// planner call could then flip many tasks at once. The rule stays inspectable at the call.
func (e *Engine) CmdPrioritiseTask(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) != 2 {
		fmt.Fprintln(out, prioritiseUsage)
		return 2, nil
	}
	id, word := args[0], args[1]
	next, known := api.ParsePriority(word)
	if !known {
		fmt.Fprintf(out, "unknown priority %q — one of: %s\n%s\n",
			word, strings.Join(api.PriorityWords, ", "), prioritiseUsage)
		return 2, nil
	}
	ps := e.store.For(c.Project)
	t, ok, err := ps.GetTask(id)
	if err != nil {
		return 1, err
	}
	if !ok {
		fmt.Fprintf(out, "no such task %q\n", id)
		return 1, nil
	}
	approval, _ := ps.GetApproval(id)
	if allowed, why := plannerMayRate(approval, t.Priority, next); !allowed {
		fmt.Fprintf(out, "%s can't be re-rated: %s. Ask the user in the meeting room.\n", id, why)
		return 1, nil
	}
	if err := e.writePriority(c.Project, id, next); err != nil {
		return 1, err
	}
	// No worker nudge, deliberately: by the rule above a planner's rating never makes a task newly
	// claimable, so a nudge here would always be waking someone for work they cannot take.
	e.refreshCachedTask(c.Project, id)
	e.deps.Notify()
	_ = ps.Log(c.Agent, "prioritise", id+" → "+word)
	fmt.Fprintf(out, "%s is now %s%s\n", id, word, claimableNote(approval))
	return 0, nil
}

// claimableNote says whether the rating actually released anything, so a planner is never left
// believing it has started work it merely ordered.
func claimableNote(approval string) string {
	if authorisedForClaim(approval) {
		return " (already approved, so this changes the order it is worked in)"
	}
	return " — still awaiting the user's approval, so no worker can take it yet"
}
