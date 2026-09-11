// package: hub/flow/agent/roles/worker / attention
// type:    logic (the worker's two attention states, declared)
// job:     where a worker goes when its work needs its attention rather than its hands: it holds
// something and has stopped doing it.
// limits:  the map. What a prod says is hub/flow/fleet's, and the mail state every role shares is
// built through one factory (-> roles/lifecycle).
package worker

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

const Stalled = "worker/stalled"

// stalled: it holds work and has stopped doing it.
var stalled = flow.State{
	Name:   Stalled,
	Title:  "Stalled over its work",
	Action: act.Prod,
	About: "The worker holds work and its screen has stopped changing past the dwell — a pane frozen " +
		"mid-turn keeps SAYING it is working for ever, so stillness rather than the word is the " +
		"evidence. It is PRODDED, not relieved: a stall is an agent that needs waking, not one that " +
		"has failed, and the work stays its own.",
	Says: says.Stalled,
	Events: flow.Events{
		{act.Done, flow.Stay, "prodded; still where it was"},
		{act.Nothing, flow.Stay, "already prodded for this spell"},
		{cond.Moving, Working, "the screen is changing again"},
		{cond.TaskGone, Idle, "the work was closed under it while it stood still"},
	},
	Verbs: flow.Offers{
		{verb.Submit, "file what you have"},
		{verb.Git, "read your changes"},
		{verb.Log, "record a note"},
		{verb.Escalate, "stop on a question"},
	},
}
