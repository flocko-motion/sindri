// package: hub / tick_reference
// type:    logic (the tick behind reference-branch drift and PR health)
// job:     re-check every project's reference branch on a slow loop, keep its open PRs
// honest against it (-> hub/flow/pr's prcheck_act.go) and against the review-row
// invariants (-> hub/flow/pr's reviewhealth_act.go), and close a meeting nobody is
// holding any more (-> chat.CloseIfIdle).
// limits:  just the cadence and lifecycle; the checks live in workflow.
package hub

import (
	"context"
	"log"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"

	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// refInterval is slow on purpose: a reference branch moves when a human does something, not on a
// machine's schedule, and each pass rebases worktrees — cheap to be late, costly to thrash.
const refInterval = 30 * time.Second

// refwatch re-checks reference branches until stopped; one per hub, started by New.
type refwatch struct {
	h *Hub
	// lastErr is the failure already reported per project. A root that isn't a git repo fails
	// every pass, and on a 30s loop that buried the log — report a change, not a drip.
	lastErr map[string]string
	// noGate is the projects already told they have no quality gate, on the same once-per-change
	// discipline: it is true until somebody edits a config file, which is not often.
	noGate map[string]bool
}

// newRefwatch builds the sweep. The registry starts it, and paces it (-> ticks.go).
func newRefwatch(h *Hub) *refwatch {
	return &refwatch{h: h, lastErr: map[string]string{}, noGate: map[string]bool{}}
}

// sweep checks each registered project. A failure is logged and the others still run — one repo
// with a broken `reference:` must not blind the hub to the rest.
func (r *refwatch) sweep(ctx context.Context) {
	projects, err := r.h.store.Projects()
	if err != nil {
		log.Printf("hub: reference check: list projects: %v", err)
		return
	}
	for _, p := range projects {
		err := r.h.prFlow().SyncReference(p.Tag)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		// Log a failure once, and again only when it changes or clears — loud without a drip.
		if msg != r.lastErr[p.Tag] {
			if msg != "" {
				log.Printf("hub: reference check for %s (%s): %s", p.Tag, p.Path, msg)
			} else {
				log.Printf("hub: reference check for %s recovered", p.Tag)
			}
			r.lastErr[p.Tag] = msg
		}
		// Said before anyone submits, rather than only when a gate refuses: a project with no
		// `verify:` cannot land ANY work, and finding that out mid-submit wastes the attempt.
		if gate := r.h.wf.VerifyCmd(p.Tag); gate == "" && !r.noGate[p.Tag] {
			r.noGate[p.Tag] = true
			log.Printf("hub: %s has no `verify:` configured — no work can be submitted from it until one is "+
				"set in .sindri/config.yaml. `verify: make check` is the usual answer; any shell command "+
				"that builds, tests and lints the project does.", p.Path)
		} else if gate != "" && r.noGate[p.Tag] {
			r.noGate[p.Tag] = false
			log.Printf("hub: %s now has a quality gate (%s).", p.Path, gate)
		}
	}
	r.preflight(ctx, projects)
	r.closeDormantMeeting()
}

// closeDormantMeeting ends a meeting that has gone quiet for an hour. On this loop rather than one
// of its own: the room is one per hub, the check is a single transcript read, and being late by a
// tick costs nothing — an hour is already the answer to "is this over?".
func (r *refwatch) closeDormantMeeting() {
	n, err := r.h.chat.CloseIfIdle()
	if err != nil {
		log.Printf("hub: closing the dormant meeting: %v", err)
		return
	}
	if n > 0 {
		log.Printf("hub: meeting closed after an hour idle — %d member(s) removed", n)
	}
}

// preflight keeps the open PRs honest against their bases, and their review rows live, OFF this loop:
// its own steps are cheap now that CheckOpenPRs only decides and queues the check (the run queue
// runs it), but the review repair and the clears here still touch git per project. Not waited on at
// shutdown — it only appends advisory history, so a write against a closed store fails harmlessly.
func (r *refwatch) preflight(ctx context.Context, projects []store.Project) {
	select {
	case <-ctx.Done():
		return // shutting down; do not start a fresh gate run
	default:
	}
	go func() {
		for _, p := range projects {
			select {
			case <-ctx.Done():
				return
			default:
			}
			r.h.prFlow().CheckOpenPRs(p.Tag)
			// A BACKSTOP for the task sources, which announce nothing: td, openspec and GitHub are
			// polled, so the cached read model only moves when something reads them. This is the one
			// place that does on its own beat; every other event reaches the decider the moment it
			// happens (-> workflowDeps.Notify), and the machine's floor re-decides regardless.
			_ = r.h.taskFlow().SyncTasks(p.Tag)
			r.h.wf.WakeProject(p.Tag, topic.TaskAvailable)
			r.h.prFlow().RepairReviewRows(p.Tag)
			// A BACKSTOP for the reviewer half, which the state machine does not yet drive in the
			// background: its reviewer states answer an ask and no more, so an unclaimed review still
			// needs a sweep to find it. The worker half needs none — a task event notifies the
			// machine, and its floor re-decides every agent regardless (-> fleet.FloorInterval).
			r.h.prFlow().AssignPendingReviews(p.Tag) // after the repair: a row it just wrote is claimable now
			// A BACKSTOP for an armed clear on an agent no state machine pass will reach: the clearing
			// state fires one for an agent standing idle, but a coauthor or a planner never lands
			// there, and arming is a direct request that must not wait on work arriving
			// (-> agent.Service.FireArmedClears).
			r.h.agents.FireArmedClears(ctx, p.Tag)
			// Idleness alone reclaims a pod, and waiting work wakes one back up — both read the fleet
			// rather than any one agent's request, so both belong on this same sweep.
			r.h.agents.FireIdleStops(ctx, p.Tag)
			r.h.agents.FireIdleStarts(ctx, p.Tag)
		}
	}()
}
