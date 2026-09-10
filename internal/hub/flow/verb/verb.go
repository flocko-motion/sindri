// package: hub/flow/verb / verb
// type:    logic (the commands an agent can run, as identities)
// job:     name every verb a state may offer, with the one line the agent reads to learn it. A verb
// belongs to the STATES that offer it, so what is possible is read off where the agent stands
// rather than re-derived by each verb for itself.
// limits:  the names and the help. Arguments and implementations are the verb's own (-> hub/verbs).
package verb

import "github.com/flo-at/sindri/internal/hub/flow"

var (
	Next       = flow.Verb{Name: "next", Help: "claim the next task: next"}
	Submit     = flow.Verb{Name: "submit", Help: "file what you have for review: submit \"<summary>\""}
	Checkpoint = flow.Verb{Name: "checkpoint", Help: "land an interim slice without finishing: checkpoint \"<summary>\""}
	Revoke     = flow.Verb{Name: "revoke", Help: "withdraw the pull request you have out: revoke"}
	Resolve    = flow.Verb{Name: "resolve", Help: "check your branch still merges, and resolve conflicts: resolve"}
	Rebase     = flow.Verb{Name: "rebase", Help: "rebase onto the current reference branch: rebase"}
	Git        = flow.Verb{Name: "git", Help: "read your changes or put a file back: git <subcommand>"}
	Task       = flow.Verb{Name: "task", Help: "read the backlog: task [<id>]"}
	Log        = flow.Verb{Name: "log", Help: "record a note on your work: log \"<note>\""}
	Fyi        = flow.Verb{Name: "fyi", Help: "one short note to the user: fyi \"<note>\""}
	Mail       = flow.Verb{Name: "mail", Help: "read your mailbox: mail"}
	Chat       = flow.Verb{Name: "chat", Help: "say something in the meeting room: chat \"<message>\""}
	Run        = flow.Verb{Name: "run", Help: "queue a slow build or test: run \"<command>\""}
	Escalate   = flow.Verb{Name: "escalate", Help: "stop on a question only the user can answer: escalate \"<question>\""}
	Resume     = flow.Verb{Name: "resume", Help: "clear your escalation once you have the answer: resume"}
	Comment    = flow.Verb{Name: "comment", Help: "comment on a task: comment <id> \"<text>\""}
	Approve    = flow.Verb{Name: "approve", Help: "approve the pull request you are reading: approve"}
	Reject     = flow.Verb{Name: "reject", Help: "send the pull request back with feedback: reject \"<why>\""}
	Scratch    = flow.Verb{Name: "scratch", Help: "check work out into a disposable workspace: scratch <ref>"}
	Plan       = flow.Verb{Name: "plan", Help: "write the plan up as a proposal: openspec submit \"<summary>\""}
	CreateTask = flow.Verb{Name: "create-task", Help: "propose a task: create-task \"<title>\" [--parent <id>]"}
	State      = flow.Verb{Name: "state", Help: "say where you are: state idle|planning"}
)

// All is every declared verb, for the check that each is offered by some state and implemented.
var All = []flow.Verb{
	Next, Submit, Checkpoint, Revoke, Resolve, Rebase, Git, Task, Log, Fyi, Mail, Chat,
	Run, Escalate, Resume, Comment, Approve, Reject, Scratch, Plan, CreateTask, State,
}
