// package: hub/situation / surface
// job:     derive, from one situation, everything the hub may do to that agent — hand it work,
// nudge it toward some, compact or clear its session, reclaim its pod, wake it — each carrying the
// reason it is refused. The one home for these rules, so no two callers can answer differently.
// type:    logic (what is allowed, decided once)
// limits:  the derivation only; performing any of it belongs to whoever asked.
package situation

import (
	"fmt"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// StallDwell is how long an agent's screen must stand completely still before the hub calls it
// stalled. It has to outlast an ordinary build or test run (a tool call froze one pane for 12s+).
const StallDwell = 3 * time.Minute

// RetryDwell is how long a cut-off turn is left before the agent is told to resume. Short because the
// pane STATES the failure, and long enough only that a retry already in flight finishes first.
const RetryDwell = time.Minute

// Surface is what may happen to an agent: each field the reason that action is refused, "" meaning
// allowed. Reasons rather than flags because they are the words the agent is told (-> Offered.Blocked).
type Surface struct {
	Assign  string // hand it work it does not have
	Nudge   string // push it toward work it could claim
	Compact string // summarise its session away
	Clear   string // discard its session
	Reclaim string // take its pod back
	Wake    string // push it anything at all
	// Stalled: it holds work and its screen has stopped moving. NeedsUser: only a human moves it on.
	Stalled   bool
	NeedsUser bool
}

// Allows reports whether an action's reason is empty, for a caller that wants the question rather
// than the words.
func Allows(reason string) bool { return reason == "" }

// Allowed derives the surface from a situation. Every rule about what may happen to an agent lives
// here; a caller that re-derives one is the drift this exists to end (-> internal/arch).
func (s Situation) Allowed() Surface {
	wake := s.wakeRefusal()
	return Surface{
		Assign:    s.assignRefusal(wake),
		Nudge:     s.nudgeRefusal(wake),
		Compact:   s.resetRefusal(),
		Clear:     s.resetRefusal(),
		Reclaim:   s.reclaimRefusal(),
		Wake:      wake,
		Stalled:   s.stalled(),
		NeedsUser: s.needsUser(),
	}
}

// AtLeafBoundary reports the agent holding nothing a reset would cut into — no leaf task, no PR
// awaiting a verdict, no review owed. A feature is not such a thing: between subtasks IS a boundary.
func (s Situation) AtLeafBoundary() bool { return Allows(s.resetRefusal()) }

// HoldsNothing counts a held feature and a human in the seat, which a leaf boundary does not. Work
// ALONE, unlike Reclaim: a retired agent holding nothing is still empty-handed.
func (s Situation) HoldsNothing() bool { return Allows(s.holdsRefusal()) }

// resetRefusal is why a clear or a compaction would cut into something. A caller inside an
// assignment it has just claimed does not ask: it knows what it put there.
func (s Situation) resetRefusal() string {
	if s.Task != "" {
		return fmt.Sprintf("holds %s — a session reset applies only at a leaf boundary", s.Task)
	}
	// Held until it MERGES: the agent owns that task until then, and a rejection returns the work
	// mid-stream, so cutting the session here loses what it was waiting for.
	if s.AwaitingPR != "" {
		return fmt.Sprintf("holds %s — %s is still to land", s.AwaitingTask, s.AwaitingPR)
	}
	if s.ReviewingPR != "" {
		return fmt.Sprintf("is reading %s — a session reset would discard the diff", s.ReviewingPR)
	}
	return ""
}

// reclaimRefusal is why the pod may not be taken back: it is already down, a human is winding the
// agent down by hand, or it holds something.
func (s Situation) reclaimRefusal() string {
	if s.Stopped {
		return "already stopped"
	}
	if s.Retired {
		return "retired by the user — the wind-down is theirs to finish, not the sweep's"
	}
	return s.holdsRefusal()
}

// holdsRefusal is why the agent is not empty-handed: any work at all, or a human in the seat. A
// coauthor is never empty-handed — its session IS the user's seat.
func (s Situation) holdsRefusal() string {
	if s.Role == "coauthor" {
		return "a coauthor's session is the user's own seat"
	}
	if s.Container != "" {
		return fmt.Sprintf("holds feature %s", s.Container)
	}
	if why := s.resetRefusal(); why != "" {
		return why
	}
	if s.Escalation != "" {
		return "escalated — waiting on the user to decide"
	}
	if s.Clients > 0 {
		return "somebody is dialed in"
	}
	return ""
}

// assignRefusal is why this agent can be handed nothing, whatever the backlog holds — the wake
// question plus the work it is already spoken for by.
func (s Situation) assignRefusal(wake string) string {
	if wake != "" {
		return wake
	}
	if s.Container != "" {
		return fmt.Sprintf("holds feature %s", s.Container)
	}
	if s.Task != "" {
		return fmt.Sprintf("holds %s", s.Task)
	}
	if s.AwaitingPR != "" {
		return fmt.Sprintf("holds %s — %s is still to land", s.AwaitingTask, s.AwaitingPR)
	}
	return ""
}

// nudgeRefusal is why pushing this agent toward claimable work would say nothing useful. Liveness is
// NOT part of it: the two sweeps read it differently on purpose, and whether a message lands is
// delivery's question.
func (s Situation) nudgeRefusal(wake string) string {
	if s.Role != "worker" {
		return "only a worker is handed backlog work"
	}
	if why := s.assignRefusal(wake); why != "" {
		return why
	}
	if s.Phase != "" && s.Phase != "idle" {
		return "mid " + s.Phase + " — not waiting on work to arrive"
	}
	return ""
}

// wakeRefusal is why waking this agent would only hand it a refusal, checked before every push. It
// mirrors the directive loop branch for branch, since the exemptions vary per branch.
func (s Situation) wakeRefusal() string {
	if s.Escalation != "" {
		return "escalated — waiting on the user to decide, not on being told there is work"
	}
	switch s.Role {
	case "coauthor", "planner":
		return "" // the directive loop checks neither retirement nor an armed clear for either
	case "reviewer":
		if s.ReviewingPR != "" && s.ReviewOpen {
			return ""
		}
		return s.parkedRefusal()
	}
	if s.Container != "" && !s.FeatureLanded {
		if s.Phase == "working" || s.Phase == "submitted" || s.Phase == "gating" {
			return ""
		}
		if s.ClearArmed { // the subtask claim gates on this alone, never on retirement
			return "a context clear is armed for it — nothing is assigned until it fires"
		}
		return ""
	}
	// No feature, or one that has landed: the directive loop drops the agent to idle and looks for
	// new work either way, so a stale "working" left over from before it landed must not exempt it.
	if s.Container == "" && (s.Phase == "working" || s.Phase == "submitted" || s.Phase == "gating") {
		return ""
	}
	if s.AwaitingPR != "" {
		return "" // its own PR to answer for, regardless of either state
	}
	return s.parkedRefusal()
}

// parkedRefusal is the pair of states a human put the agent in, in the order the directive loop
// checks them.
func (s Situation) parkedRefusal() string {
	if s.Retired {
		return fmt.Sprintf("retired by the user — `sindri agent retire %s --back` brings it back", s.Name)
	}
	if s.ClearArmed {
		return "a context clear is armed for it — nothing is assigned until it fires"
	}
	return ""
}

// stalled reports an agent holding work it has stopped doing. The evidence is the SCREEN standing
// still — a pane frozen mid-turn keeps SAYING "working" for ever. Three states veto it, each one an
// agent correctly motionless: blocked, signed-out, and queued behind the fleet's own gate.
func (s Situation) stalled() bool {
	// A cut-off turn counts in ANY phase: nothing resumes on its own, and an agent that could not
	// finish its own sentence will not act on a verdict either.
	if s.Runtime == "api-error" {
		return s.StillFor >= RetryDwell
	}
	if s.Runtime == "blocked" || s.Runtime == "signed-out" || s.WaitingOnRun || s.StillFor < StallDwell {
		return false
	}
	// "submitted" and "gating" exist to wait; reviewing does not — a reviewer with a PR is meant to
	// be reading it.
	return s.Phase == "working" || s.Phase == "reviewing" ||
		(s.Container != "" && s.Phase != "submitted" && s.Phase != "gating")
}

// needsUser reports a state only a human resolves, read off the word the board shows so the marker
// and the status cannot disagree. The stall is folded in HERE: it is this surface's own verdict, and
// a Status arriving with it would have the two reading each other (-> hub.situationObserver).
func (s Situation) needsUser() bool {
	status := s.Status
	if s.stalled() {
		status = api.StatusStalled
	}
	return api.AgentNeedsUser(api.AgentView{Status: status, Retired: s.Retired})
}

// ParkedByTheHub reports an agent idle because it was TOLD to be — retired and empty-handed, or a
// feature worker whose next subtask is gated on the user. Prodding either complains about the hub.
func (s Situation) ParkedByTheHub() bool {
	if s.Retired {
		// Only once it holds nothing: retirement is "no new work", and finishing what it already has
		// requires the verdicts about that work to keep reaching it.
		return s.HoldsNothing()
	}
	if s.Container == "" || s.Task != "" || s.Phase != "idle" {
		return false
	}
	return len(s.Pool.GatedUnder(s.Container)) > 0
}
