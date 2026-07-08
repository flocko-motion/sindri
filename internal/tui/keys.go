// package: tui / keys
// type:    ui (keymap — single source of truth)
// job:     the one place every hotkey is declared: its key, its help label, and the
//          scope it belongs to. onKey dispatches on the key constants defined here,
//          and the footers are generated from the keymap table — so a binding and
//          its help can never drift apart.
// limits:  declaration + help rendering only; what each action does lives in onKey.
package tui

import "strings"

// Action-key constants. onKey switches on these and the keymap table lists them, so
// changing a key (or discovering a conflict) is a single edit here. Navigation keys
// that are compound in the help (tab, pane, move) are dispatched by their literal
// tea strings in onKey and appear in the keymap only as display rows.
const (
	keyNew      = "N" // new task / new agent
	keyEdit     = "e" // edit task fields / agent memory
	keyPriority = "P" // set task priority (mutation → shift)
	keyUnassign = "U" // release a task to the backlog
	keyClose    = "C" // close a task
	keyApprove  = "A" // agentic review (prs) / approve proposed task (tasks)
	keyReject   = "R" // reject a PR / a proposed task
	keyStartS   = "S" // agent start/stop
	keyTell     = "t" // tell an agent / show a PR's task
	keyAttachAp = "a" // attach (agents) / approve PR (prs)
	keyMerge    = "m" // merge a PR
	keyDelete   = "D" // delete an agent
	keyLint     = "L" // lint a PR
	keyVerify   = "V" // verify (materialize) a PR
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
	{"⇥/⇧⇥", lbl("tab"), scopeGlobal},
	{"C-h/l", lbl("pane"), scopeGlobal},
	{"j/k", lbl("move"), scopeGlobal},
	{keyDetail, lbl("detail"), scopeGlobal},
	{keyRepo, lbl("repo"), scopeGlobal},
	{keyConfig, lbl("config"), scopeGlobal},
	{keyRefresh, lbl("refresh"), scopeGlobal},
	{keyQuit, lbl("quit"), scopeGlobal},

	// Tasks.
	{keyNew, lbl("new"), scopeTasks},
	{keyEdit, lbl("edit"), scopeTasks},
	{keyPriority, lbl("priority"), scopeTasks},
	{keyUnassign, lbl("unassign"), scopeTasks},
	{keyClose, lbl("close"), scopeTasks},
	{keyDelete, lbl("scrap"), scopeTasks},
	{"A/R", lbl("approve/reject"), scopeTasks},
	{keyFilter, func(m model) string { return "filter: " + filterNames[m.filter] }, scopeTasks},

	// Agents.
	{keyNew, lbl("new"), scopeAgents},
	{keyStartS, lbl("start/stop"), scopeAgents},
	{keyTell, lbl("tell"), scopeAgents},
	{keyAttachAp, lbl("attach"), scopeAgents},
	{keyEdit, lbl("memory"), scopeAgents},
	{keyReject, lbl("rebase"), scopeAgents}, // R = reBase (onto the reference branch)
	{keyDelete, lbl("delete"), scopeAgents},
	{keyScopeTog, func(m model) string { return "scope: " + scopeName(m.scopeRepo[1]) }, scopeAgents},

	// PRs.
	{keyVerify, lbl("verify"), scopePRs},
	{keyAttachAp, lbl("approve"), scopePRs},
	{keyReject, lbl("reject"), scopePRs},
	{keyApprove, lbl("agent-review"), scopePRs},
	{keyLint, lbl("lint"), scopePRs},
	{keyMerge, lbl("merge"), scopePRs},
	{keyFilter, func(m model) string { return "filter: " + prFilterNames[m.prFilter] }, scopePRs},
	{keyScopeTog, func(m model) string { return "scope: " + scopeName(m.scopeRepo[2]) }, scopePRs},

	// Repos.
	{"enter", lbl("switch"), scopeRepos},
	{keyColor, lbl("colour"), scopeRepos},
	{keyConfig, lbl("config"), scopeRepos},
	{keyDelete, lbl("forget"), scopeRepos},
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
	default:
		return scopeRepos
	}
}
