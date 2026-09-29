// package: hub/api/agents/verb / verb
// type:    logic (every verb an agent can run, declared once)
// job:     the catalogue — one Def per verb, carrying the name it is typed under, the line a
// state's offer list prints, the roles it belongs to, and whether the flow machine governs
// it. The single answer to "what verbs are there", read by the state maps, the registry
// and the directive alike.
// limits:  data only, so a flow declaration may import it (-> internal/arch/flow_test.go). What a
// verb DOES, its full usage text and the gates on it are bound one level up
// (-> hub/api/agents), beside the implementation each belongs to.
package verb

import (
	"fmt"

	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

// Def is one verb, as everything that lists verbs sees it.
type Def struct {
	// Name is what the agent types, and the key the binding table answers to.
	Name string
	// Summary is the line a state's offer list prints (-> fleet.offerLines). Short and imperative,
	// because it is read in a column beside a dozen others. The full usage lives with the verb's
	// implementation and is bound to it by name.
	Summary string
	// Roles that may see and run it; empty means every role.
	Roles []string
	// Governed marks a verb the flow machine decides, so its availability is read off where the
	// agent stands rather than from its own closure (-> registry.Standing.Refuses). A verb outside
	// this has not changed hands yet.
	Governed bool
}

// Catalogue is every verb there is, in the order the surface lists them, and the ONE literal each
// is written in. The binding table is checked against this set both ways, so a verb here with
// nothing behind it — and an implementation under a name nothing declares — are both failures
// caught at test time rather than by an agent meeting a dead end.
var Catalogue = []Def{
	{Name: "status", Summary: "show who you are and where you stand: status"},
	{Name: "log", Summary: "record a note on your work: log \"<note>\"", Governed: true},
	{Name: "prs", Summary: "list pull requests and their status: prs [--limit N]"},
	{Name: "show", Summary: "read a pull request's diff or a run's output: show <id>"},
	{Name: "lint", Summary: "run the quality gate: lint [<pr-id>]"},
	{Name: "submit", Summary: "file what you have for review: submit \"<summary>\"", Roles: []string{"worker"}, Governed: true},
	{Name: "contribute", Summary: "land an interim contribution mid-task: contribute \"<summary>\"", Roles: []string{"worker"}},
	{Name: "revoke", Summary: "withdraw the pull request you have out: revoke", Roles: []string{"worker", "planner"}, Governed: true},
	{Name: "resolve", Summary: "check your branch still merges, and resolve conflicts: resolve", Roles: []string{"worker"}, Governed: true},
	{Name: "rebase", Summary: "rebase onto the current reference branch: rebase", Roles: []string{"worker", "planner"}, Governed: true},
	{Name: "git", Summary: "read your changes or put a file back: git <subcommand>", Roles: []string{"worker", "planner", "coauthor"}, Governed: true},
	{Name: "scratch", Summary: "check work out into a disposable workspace: scratch <ref>", Roles: []string{"coauthor"}, Governed: true},
	{Name: "run", Summary: "queue a slow build or test: run \"<command>\"", Roles: []string{"worker", "reviewer", "coauthor"}, Governed: true},
	{Name: "checkpoint", Summary: "record the current subtask and move to the next: checkpoint \"<summary>\"", Roles: []string{"worker"}, Governed: true},
	{Name: "task", Summary: "read the backlog: task [<id>]", Roles: []string{"planner", "coauthor", "worker", "reviewer"}, Governed: true},
	{Name: "create-task", Summary: "propose a task: create-task \"<title>\" [--parent <id>]", Roles: []string{"planner", "coauthor"}, Governed: true},
	{Name: "edit-task", Summary: "revise a task, returning it for re-approval: edit-task <id>", Roles: []string{"planner", "coauthor"}},
	{Name: "prioritise-task", Summary: "set the order of work you planned: prioritise-task <id> <rating>", Roles: []string{"planner"}},
	{Name: "reopen-task", Summary: "reopen a closed task, with a reason: reopen-task <id> \"<why>\"", Roles: []string{"planner"}},
	// Named for the verb it is TYPED under. It was declared as "plan" while the registry served
	// "openspec", so every planner was offered a verb that answered "unknown".
	{Name: "openspec", Summary: "ship your spec edits as a pull request: openspec submit \"<summary>\"", Roles: []string{"planner"}, Governed: true},
	{Name: "staff", Summary: "list this repo's agents and what each is working on: staff", Roles: []string{"planner"}},
	{Name: "comment", Summary: "comment on a task: comment <id> \"<text>\"", Roles: []string{"worker", "reviewer", "planner", "coauthor"}, Governed: true},
	{Name: "approve", Summary: "approve the pull request you are reading: approve", Roles: []string{"reviewer", "planner", "coauthor"}, Governed: true},
	{Name: "reject", Summary: "send the pull request back with feedback: reject \"<why>\"", Roles: []string{"reviewer", "coauthor"}, Governed: true},
	{Name: "escalate", Summary: "stop on a question only the user can answer: escalate \"<question>\"", Governed: true},
	{Name: "resume", Summary: "clear your escalation once you have the answer: resume", Governed: true},
	{Name: "fyi", Summary: "one short note to the user: fyi \"<note>\"", Roles: []string{"worker", "reviewer", "planner"}, Governed: true},
	{Name: "mail", Summary: "read your mailbox: mail", Governed: true},
	{Name: "reply", Summary: "answer a message you were sent: reply <mail-id> \"<message>\""},
	// Named for the verb it is TYPED under, for the same reason as openspec: this was declared as
	// "chat" while the registry served "meeting", in four role maps at once.
	{Name: "meeting", Summary: "say something in the meeting room: meeting \"<message>\"", Governed: true},
}

// The offer values a state map names. Derived from the catalogue rather than written again, so a
// map and the registry cannot come to serve different names for one verb — which is exactly what
// they had done for `chat` and `plan`.
var (
	Status         = of("status")
	Log            = of("log")
	PRs            = of("prs")
	Show           = of("show")
	Lint           = of("lint")
	Submit         = of("submit")
	Contribute     = of("contribute")
	Revoke         = of("revoke")
	Resolve        = of("resolve")
	Rebase         = of("rebase")
	Git            = of("git")
	Scratch        = of("scratch")
	Run            = of("run")
	Checkpoint     = of("checkpoint")
	Task           = of("task")
	CreateTask     = of("create-task")
	EditTask       = of("edit-task")
	PrioritiseTask = of("prioritise-task")
	ReopenTask     = of("reopen-task")
	Openspec       = of("openspec")
	Staff          = of("staff")
	Comment        = of("comment")
	Approve        = of("approve")
	Reject         = of("reject")
	Escalate       = of("escalate")
	Resume         = of("resume")
	Fyi            = of("fyi")
	Mail           = of("mail")
	Reply          = of("reply")
	Meeting        = of("meeting")
)

// of is the catalogue row for name, as the machine offers it. It panics on a name the catalogue
// does not carry: this runs at package init, so a typo here fails every test in the tree at once
// rather than one agent's directive at run time.
func of(name string) machine.Verb {
	d, ok := Lookup(name)
	if !ok {
		panic(fmt.Sprintf("verb %q is offered but not in the catalogue", name))
	}
	return machine.Verb{Name: d.Name, Help: d.Summary}
}

// Lookup is the catalogue row for name, ok=false when there is none.
func Lookup(name string) (Def, bool) {
	for _, d := range Catalogue {
		if d.Name == name {
			return d, true
		}
	}
	return Def{}, false
}

// All is the verbs the flow machine governs — the list that decides whether a state's silence
// about a verb is a refusal (-> registry.Standing.Governs). Derived, never listed a second time.
var All = governed()

func governed() []machine.Verb {
	out := make([]machine.Verb, 0, len(Catalogue))
	for _, d := range Catalogue {
		if d.Governed {
			out = append(out, machine.Verb{Name: d.Name, Help: d.Summary})
		}
	}
	return out
}
