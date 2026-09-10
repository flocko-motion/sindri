// package: hub/flow/roles/worker / attention
// type:    logic (the worker's two attention states, declared)
// job:     the two places a worker goes when something needs its attention rather than its hands —
// mail it has not read, and work it has stopped doing.
// limits:  the map. What a prod says, and what mail is served, is hub/flow/roles'.
package worker

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
	"github.com/flo-at/sindri/internal/hub/flow/agent/verb"
)

const (
	Mail    = "worker/mail"
	Stalled = "worker/stalled"
)

// mail: it has messages it has not read, so it is not done, so it is not prepared for anything else.
var mail = flow.State{
	Name:  Mail,
	Title: "Mail to read",
	About: "The worker has unread mail. Unread mail means it is NOT DONE — and an agent that is not " +
		"done is never prepared for new work, which is what keeps a clear from landing on top of a " +
		"message nobody has seen. The mail is served with the directive; reading it is the exit.",
	Says: says.Mail,
	Events: flow.Events{
		{cond.MailRead, Idle, "the mailbox is empty again"},
	},
	Verbs: flow.Offers{
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

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
		{verb.Submit, Submitting, "file what you have"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}
