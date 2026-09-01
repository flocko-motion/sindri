// package: hub/observe / observe
// type:    logic (what the harness can see of one agent's box)
// job:     carry the harness's evidence across the seam as one value, stamped with when it was
// taken — so what the hub SAW can be shown beside what it CONCLUDED. The session's own account of
// itself crosses as a TYPE, never a word (-> state.go).
// limits:  evidence only. Whether that adds up to stalled, idle or needing a human depends on the
// work the agent holds, and belongs to the orchestrator (-> hub/situation.Surface).
package observe

import "time"

// Observation is what the harness saw of one agent's box at a moment. Every field is evidence; not
// one of them is a judgement about the work the agent holds.
type Observation struct {
	// TakenAt is when this was sampled. Zero means nothing has looked yet — a different claim from
	// "down", and reading the two as one is how a freshly registered agent came to be reported dead.
	TakenAt time.Time
	Up      bool
	Clients int // humans attached to the session
	// State is what the session says it is doing, parsed at the boundary into a value (-> ParseState).
	// A caller asks the predicates below; there is no word here to match against.
	State State
	// Digest is a hash of what the display shows, so stillness is measurable. "" when the look
	// failed, so a lost one never reads as "nothing changed".
	Digest string
	// StillSince is when the display last changed; ToolSince when an in-flight tool call was first
	// seen this streak; StateSince when the session's own account of itself last changed.
	StillSince time.Time
	ToolSince  time.Time
	StateSince time.Time
	// Fill and Window size the session's context, Model names what it runs on.
	Fill   int
	Window int
	Model  string
	// The transient intent between a start or a stop being ASKED FOR and the process being seen to
	// follow — the box's own state, not the workflow's.
	Launching    bool
	LaunchFailed bool
	Stopping     bool
}

// Seen reports whether anything has looked at this agent yet.
func (o Observation) Seen() bool { return !o.TakenAt.IsZero() }

// AtPrompt reports the session sitting at an empty prompt. An observation about the SCREEN: whether
// that means the agent is idle — free to be handed work — depends on what it holds, and is the
// orchestrator's to say.
func (o Observation) AtPrompt() bool { return o.State == AtPrompt }

// Working reports the session saying a turn is running. It keeps saying so through a wedged turn,
// which is why stillness rather than this word is what a stall is measured by.
func (o Observation) Working() bool { return o.State == Working }

// AwaitingHuman reports the session showing a question at its prompt — it cannot proceed until
// somebody answers in the pane.
func (o Observation) AwaitingHuman() bool { return o.State == AwaitingHuman }

// SignedOut reports the pane carrying a /login banner, so nothing typed there is sent.
func (o Observation) SignedOut() bool { return o.State == SignedOut }

// TurnCutOff reports the session saying its turn died mid-stream. Nothing resumes one on its own.
func (o Observation) TurnCutOff() bool { return o.State == TurnCutOff }

// StillFor is how long the display has stood unchanged as of now, zero when nothing has been seen.
// A cut-off turn is timed from when it started SAYING so: its spinner keeps redrawing, so the screen
// never stands still and a stillness clock would restart for ever.
func (o Observation) StillFor(now time.Time) time.Duration {
	since := o.StillSince
	if o.TurnCutOff() {
		since = o.StateSince
	}
	if !o.Up || since.IsZero() {
		return 0
	}
	return now.Sub(since)
}
