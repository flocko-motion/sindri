// package: tui / keys
// type:    ui (keymap — single source of truth)
// job:     the one place every hotkey is declared: its key, its help label, and the
// scope it belongs to. onKey dispatches on the key constants defined here,
// and the footers are generated from the keymap table — so a binding and
// its help can never drift apart.
// limits:  declaration + help rendering only; what each action does lives in onKey.
package tui

import "strings"

// Action-key constants. onKey switches on these and the keymap table lists them, so
// changing a key (or discovering a conflict) is a single edit here. Navigation keys
// that are compound in the help (tab, pane, move) are dispatched by their literal
// tea strings in onKey and appear in the keymap only as display rows.
//
// CASE IS THE CONVENTION: lowercase looks or navigates, uppercase changes something. The binding
// direction that carries the safety is lowercase: a mistyped one must never commit a change. It may
// OPEN a form, chooser or prompt that then commits, since that flow is confirmable and esc cancels.
//
// The vim-family view keys (J/K scroll, G bottom, Y yank) are uppercase and mutate nothing; they
// keep the shape a terminal user already has in their fingers.
const (
	keyNew      = "N" // new task / new agent
	keyBrief    = "B" // hand a task to a planner to work up (brief)
	keyEdit     = "e" // edit the selection: task fields (tasks) / open the workspace in $EDITOR (agents, prs)
	keyOptions  = "O" // an agent's options (mutation → shift)
	keyPriority = "P" // set task priority (mutation → shift)
	keyUnassign = "U" // release a task to the backlog
	keyClose    = "C" // close a task
	keyApprove  = "A" // approve: a PR (prs) / a proposed task (tasks) — one letter, one meaning
	keyReview   = "I" // invite an agentic review of a PR (A is approve, R is reject)
	keyReject   = "R" // reject a PR / a proposed task
	keyStartS   = "S" // agent start/stop
	keyTell     = "t" // tell an agent / show a PR's task
	keyAttach   = "a" // attach to an agent's session (agents, tasks, prs)
	keyMerge    = "M" // merge a PR — commits on the keystroke, so it takes the mutation case
	keyDelete   = "D" // delete an agent
	keyLint     = "L" // lint a PR
	keyVerify   = "V" // verify (materialize) a PR
	keyOpen     = "o" // open the row's worktree in a shell (navigation → lowercase)
	keyFilter   = "f" // cycle the tasks filter
	keyScopeTog = "s" // toggle a tab's global↔repo scope
	keyRepo     = "p" // switch the active repo/project (navigation → lowercase)
	keyConfig   = "E" // edit the repo's config (mutation → shift)
	keyColor    = "c" // pick a repo's colour (opens a chooser → lowercase)
	keyRefresh  = "r" // refresh the board
	keyDetail   = "§" // toggle the detail pane
	keyQuit     = "q" // quit
)

// keyScope selects where a binding applies and is shown.
type keyScope int

const (
	scopeGlobal keyScope = iota // shown on every tab
	scopeTasks
	scopeAgents
	scopePRs
	scopeRepos
	scopeChat
)

// binding is one row of help: the key(s) as displayed, a label (may depend on model
// state, e.g. the active filter), and the scope that decides where it shows.
type binding struct {
	keys  string
	label func(m model) string
	scope keyScope
}

// lbl wraps a static label.
func lbl(s string) func(model) string { return func(model) string { return s } }

// keymap is the single source of truth for the actionable hotkeys shown in the
// footers. Order here is the order shown.
var keymap = []binding{
	// Global (first footer row): the compound nav keys are display-only rows.
	{"⇥/[]", lbl("tab"), scopeGlobal},
	{"C-h/l", lbl("pane"), scopeGlobal},
	{"j/k", lbl("move"), scopeGlobal},
	{keyDetail, lbl("detail"), scopeGlobal},
	{keyRepo, lbl("repo"), scopeGlobal},
	{keyConfig, lbl("config"), scopeGlobal},
	{keyRefresh, lbl("refresh"), scopeGlobal},
	{keyQuit, lbl("quit"), scopeGlobal},

	// Tasks: each scope's rows are grouped and ordered look-first, so the footer reads left to
	// right from the harmless to the decisive.
	{keyNew, lbl("new"), scopeTasks},
	{keyBrief, lbl("brief a planner"), scopeTasks},
	{keyAttach, lbl("attach"), scopeTasks},
	{keyEdit, lbl("edit"), scopeTasks},
	{keyPriority, lbl("priority"), scopeTasks},
	{keyUnassign, lbl("unassign"), scopeTasks},
	{keyClose, lbl("close"), scopeTasks},
	{keyDelete, lbl("scrap"), scopeTasks},
	{"A/R", lbl("approve/reject"), scopeTasks},
	{keyFilter, func(m model) string { return "filter: " + filterNames[m.filter] }, scopeTasks},

	// Agents.
	{keyNew, lbl("new"), scopeAgents},
	{keyTell, lbl("tell"), scopeAgents},
	{keyAttach, lbl("attach"), scopeAgents},
	{keyEdit, lbl("editor"), scopeAgents},
	{keyOpen, lbl("open"), scopeAgents},
	{keyStartS, lbl("start/stop"), scopeAgents},
	{keyOptions, lbl("options"), scopeAgents},
	{keyReject, lbl("rebase"), scopeAgents}, // R = reBase (onto the reference branch)
	{keyDelete, lbl("delete"), scopeAgents},
	{keyScopeTog, func(m model) string { return "scope: " + scopeName(m.scopeRepo) }, scopeAgents},

	// PRs: look (verify/editor/open/lint), then the verdicts, then merge.
	{keyVerify, lbl("verify"), scopePRs},
	{keyEdit, lbl("editor"), scopePRs},
	{keyOpen, lbl("open"), scopePRs},
	{keyAttach, lbl("attach"), scopePRs},
	{keyLint, lbl("lint"), scopePRs},
	{keyApprove, lbl("approve"), scopePRs},
	{keyReject, lbl("reject"), scopePRs},
	{keyReview, lbl("agent-review"), scopePRs},
	{keyMerge, lbl("merge"), scopePRs},
	{keyDelete, lbl("scrap"), scopePRs},
	{keyFilter, func(m model) string { return "filter: " + prFilterNames[m.prFilter] }, scopePRs},
	{keyScopeTog, func(m model) string { return "scope: " + scopeName(m.scopeRepo) }, scopePRs},

	// Repos.
	{"enter", lbl("switch"), scopeRepos},
	{keyColor, lbl("colour"), scopeRepos},
	{keyConfig, lbl("config"), scopeRepos},
	{keyDelete, lbl("forget"), scopeRepos},

	// Chat (membership is curated from the CLI: `sindri meeting add/remove`).
	{"enter", lbl("compose"), scopeChat},
	{keyNew, lbl("new meeting"), scopeChat},
}

// footerFor renders the "key label · key label" hints for a scope from the keymap.
func (m model) footerFor(scope keyScope) string {
	var parts []string
	for _, b := range keymap {
		if b.scope == scope {
			parts = append(parts, b.keys+" "+b.label(m))
		}
	}
	return strings.Join(parts, " · ")
}

// tabScope maps a tab index to its keymap scope.
func tabScope(tab int) keyScope {
	switch tab {
	case 0:
		return scopeTasks
	case 1:
		return scopeAgents
	case 2:
		return scopePRs
	case 4:
		return scopeChat
	default:
		return scopeRepos
	}
}
