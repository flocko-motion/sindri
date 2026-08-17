// package: tui / keys
// type:    ui (keymap — single source of truth)
// job:     the one place every hotkey is declared: its key, its help label, and the
// scope it belongs to. onKey dispatches on the key constants defined here,
// and the footers are generated from the keymap table — so a binding and
// its help can never drift apart.
// limits:  declaration + help rendering only; what each action does lives in onKey.
package tui

import "strings"

// Action-key constants: onKey switches on these and the keymap lists them, so a rebinding is one
// edit. Compound navigation rows (tab, pane, move) are display-only.
//
// NAVIGATE OR COMMIT, with no exception clause. Navigating is direct, and opening an editor, form
// or picker IS navigating — the act happens on submit inside the place the key took you to. A
// confirm is not such a place: it asks "sure?" about an action the keystroke has already chosen, so
// it belongs to the commit. Committing is behind the space prefix (keyMenu), which makes those
// letters live and leaves them inert otherwise; M is still merge. Case only hints (J/K/G/Y look,
// N/O/P/E open): the `commits` field decides, per binding and per tab, since the same letter can
// navigate on one tab and commit on another.
const (
	keyNew       = "N" // new task / new agent
	keyBrief     = "B" // hand a task to a planner to work up (brief)
	keyEdit      = "e" // edit the selection: task fields (tasks) / open the workspace in $EDITOR (agents, prs)
	keyOptions   = "O" // agents: an agent's options · tasks: reOpen a closed task — both open a form
	keyPriority  = "P" // set task priority — opens a picker, so it stays direct
	keyUnassign  = "U" // release a task to the backlog
	keyClose     = "C" // close a task
	keyClearCtx  = "C" // clear a full agent's context — C ends what is in front of you, as it does on a task
	keyCloseMeet = "C" // close the meeting — the same "end what is in front of you" on the Chat tab
	keyApprove   = "A" // approve: a PR (prs) / a proposed task (tasks) — one letter, one meaning
	keyReview    = "I" // invite an agentic review of a PR (A is approve, R is reject)
	keyReject    = "R" // reject a PR / a proposed task
	keyStartS    = "S" // agent start/stop
	keyMilestone = "M" // capture an agent's container branch as a PR — confirms, then commits
	keyRebuild   = "B" // reBuild the agent image and relaunch — confirms, then commits
	keyRetire    = "X" // wind an agent down: no new work, on the keystroke. R is rebase here, S stops it
	keyStats     = "m" // an agent's memory against its limit (a view)
	keyTell      = "t" // tell an agent / show a PR's task
	keyComment   = "i" // comment on a task — opens a prompt
	keyAttach    = "a" // attach to an agent's session (agents, tasks, prs)
	keyMerge     = "M" // merge a PR — commits on the keystroke, which is why it sits behind the prefix
	keyDelete    = "D" // delete an agent
	keyLint      = "L" // lint a PR
	keyVerify    = "V" // verify (materialize) a PR
	keyOpen      = "o" // open the row's worktree in a shell
	keyFilter    = "f" // cycle the tasks filter
	keyWhyNext   = "n" // what would be assigned next, and why nothing else would be (a view)
	keyScopeTog  = "s" // toggle a tab's global↔repo scope
	keyRepo      = "p" // switch the active repo/project
	keyConfig    = "E" // edit the repo's config — opens a form, so it stays direct
	keyColor     = "c" // pick a repo's colour — opens a picker
	keyRefresh   = "r" // refresh the board
	// Mail: narrow the list to the selected message's recipient — "who was told this?", the question
	// the tab is opened with. Its own letter because `a` attaches and only attaches, on every tab.
	keyMailWho = "w"
	keyMenu    = " " // the prefix: opens the menu of committing actions for the selected row
	// keyMenuShown is how the prefix reads in a footer: a bare space would render as a gap, and a
	// gap advertises nothing.
	keyMenuShown = "space"
	keyDetail    = "§" // toggle the detail pane
	keyQuit      = "q" // quit
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
	scopeRuns
	scopeMail
)

// binding is one row of help: the displayed key(s), a label (which may read the model), the scope
// it shows in, and whether it COMMITS — the fact that puts it behind the prefix. `when` narrows a
// committing action to the rows it applies to (nil = always), so what would be refused is never
// offered, as the hub does for an agent's command surface.
type binding struct {
	keys    string
	label   func(m model) string
	scope   keyScope
	commits bool
	when    func(m model) bool
}

// lbl wraps a static label.
func lbl(s string) func(model) string { return func(model) string { return s } }

// keymap is the single source of truth for the actionable hotkeys shown in the
// footers. Order here is the order shown.
var keymap = []binding{
	// Global (first footer row): the compound nav keys are display-only rows. "C-h/C-l", not
	// "C-h/l" — the trailing bare "l" would otherwise be misread as the plain letter, which IS a
	// real, different binding (tasks: expand a fold) and must not share this row's label.
	{keys: "⇥/[]", label: lbl("tab"), scope: scopeGlobal},
	{keys: "C-h/C-l", label: lbl("pane"), scope: scopeGlobal},
	{keys: "j/k/g/G", label: lbl("move/top/bot"), scope: scopeGlobal},
	{keys: "J/K", label: lbl("scroll detail"), scope: scopeGlobal},
	// "page", not "scroll detail": ctrl+d/ctrl+u half-page whichever column has the focus — the
	// list on the left, the detail (or the PRs meta column) on the right — so a label naming one
	// pane would be wrong half the time.
	{keys: "C-d/C-u", label: lbl("page"), scope: scopeGlobal},
	{keys: "y/Y", label: lbl("yank/all"), scope: scopeGlobal},
	{keys: keyDetail, label: lbl("detail"), scope: scopeGlobal},
	{keys: keyRepo, label: lbl("repo"), scope: scopeGlobal},
	{keys: keyConfig, label: lbl("config"), scope: scopeGlobal},
	{keys: keyRefresh, label: lbl("refresh"), scope: scopeGlobal},
	{keys: keyQuit, label: lbl("quit"), scope: scopeGlobal},

	// Tasks: each scope's rows are grouped and ordered look-first, so the footer reads left to
	// right from the harmless to the decisive.
	{keys: keyNew, label: lbl("new"), scope: scopeTasks},
	{keys: "h/l", label: lbl("fold"), scope: scopeTasks},
	{keys: keyBrief, label: lbl("brief a planner"), scope: scopeTasks},
	{keys: keyComment, label: lbl("comment"), scope: scopeTasks},
	{keys: keyAttach, label: lbl("attach"), scope: scopeTasks},
	{keys: keyEdit, label: lbl("edit"), scope: scopeTasks},
	{keys: keyPriority, label: lbl("priority"), scope: scopeTasks},
	{keys: keyUnassign, label: lbl("unassign"), scope: scopeTasks, commits: true, when: taskHeld},
	{keys: keyClose, label: lbl("close"), scope: scopeTasks, commits: true, when: taskOpen},
	{keys: keyOptions, label: lbl("reopen"), scope: scopeTasks},
	{keys: keyDelete, label: lbl("scrap"), scope: scopeTasks, commits: true},
	{keys: keyApprove, label: lbl("approve"), scope: scopeTasks, commits: true, when: taskAwaitsVerdict},
	{keys: keyReject, label: lbl("reject"), scope: scopeTasks, when: taskAwaitsVerdict},
	{keys: keyWhyNext, label: lbl("why next"), scope: scopeTasks},
	{keys: keyFilter, label: func(m model) string { return "filter: " + string(m.filter) }, scope: scopeTasks},

	// Agents.
	{keys: keyNew, label: lbl("new"), scope: scopeAgents},
	{keys: keyTell, label: lbl("tell"), scope: scopeAgents},
	{keys: keyAttach, label: lbl("attach"), scope: scopeAgents},
	{keys: keyEdit, label: lbl("editor"), scope: scopeAgents},
	{keys: keyOpen, label: lbl("open"), scope: scopeAgents},
	{keys: keyStartS, label: lbl("start/stop"), scope: scopeAgents, commits: true},
	{keys: keyOptions, label: lbl("options"), scope: scopeAgents},
	{keys: keyStats, label: lbl("stats"), scope: scopeAgents},
	{keys: keyMilestone, label: lbl("milestone PR"), scope: scopeAgents, commits: true},
	{keys: keyRebuild, label: lbl("rebuild image"), scope: scopeAgents, commits: true},
	{keys: keyReject, label: lbl("rebase"), scope: scopeAgents, commits: true}, // R = reBase (onto the reference branch)
	// The label tracks the selection, since the key toggles and "retire" on an already-retired
	// agent reads as a no-op the user would not press.
	{keys: keyRetire, label: func(m model) string {
		if a, ok := m.selAgent(); ok && a.Retired {
			return "unretire"
		}
		return "retire"
	}, scope: scopeAgents, commits: true},
	{keys: keyClearCtx, label: func(m model) string {
		if a, ok := m.selAgent(); ok && a.ClearArmed {
			return "cancel clear"
		}
		return "clear context"
	}, scope: scopeAgents, commits: true},
	{keys: keyDelete, label: lbl("delete"), scope: scopeAgents, commits: true},
	{keys: keyScopeTog, label: func(m model) string { return "scope: " + scopeName(m.scopeRepo) }, scope: scopeAgents},

	// PRs: look (verify/editor/open/lint), then the verdicts, then merge.
	{keys: keyVerify, label: lbl("verify"), scope: scopePRs, commits: true},
	{keys: keyEdit, label: lbl("editor"), scope: scopePRs},
	{keys: keyOpen, label: lbl("open"), scope: scopePRs},
	{keys: keyAttach, label: lbl("attach"), scope: scopePRs},
	{keys: keyLint, label: lbl("lint"), scope: scopePRs, commits: true},
	{keys: keyApprove, label: lbl("approve"), scope: scopePRs, commits: true, when: prDecidable},
	{keys: keyReject, label: lbl("reject"), scope: scopePRs},
	{keys: keyReview, label: lbl("agent-review"), scope: scopePRs},
	{keys: keyMerge, label: lbl("merge"), scope: scopePRs, commits: true, when: model.selPRApproved},
	{keys: keyDelete, label: lbl("scrap"), scope: scopePRs, commits: true},
	{keys: keyFilter, label: func(m model) string { return "filter: " + string(m.prFilter) }, scope: scopePRs},
	{keys: keyScopeTog, label: func(m model) string { return "scope: " + scopeName(m.scopeRepo) }, scope: scopePRs},

	// Repos.
	{keys: "enter", label: lbl("switch"), scope: scopeRepos},
	{keys: keyColor, label: lbl("colour"), scope: scopeRepos},
	{keys: keyConfig, label: lbl("config"), scope: scopeRepos, commits: true},
	{keys: keyDelete, label: lbl("forget"), scope: scopeRepos, commits: true},

	// Mail: look only — the mailbox is the agent's to read, and the user's part is finding a message.
	{keys: keyAttach, label: lbl("attach"), scope: scopeMail},
	{keys: keyMailWho, label: func(m model) string {
		if m.mailAgent != "" {
			return "who: " + m.mailAgent
		}
		return "who: all"
	}, scope: scopeMail},
	{keys: keyFilter, label: func(m model) string { return "filter: " + string(m.mailFilter) }, scope: scopeMail},
	{keys: keyScopeTog, label: func(m model) string { return "scope: " + scopeName(m.scopeRepo) }, scope: scopeMail},

	// Chat.
	{keys: "enter", label: lbl("compose"), scope: scopeChat},
	{keys: keyApprove, label: lbl("add member"), scope: scopeChat},
	{keys: keyReject, label: lbl("remove member"), scope: scopeChat},
	{keys: keyNew, label: lbl("new meeting"), scope: scopeChat, commits: true},
	{keys: keyCloseMeet, label: lbl("close meeting"), scope: scopeChat, commits: true},

	// Runs.
	{keys: keyNew, label: lbl("queue a run"), scope: scopeRuns}, // opens a prompt to type the command in
	{keys: keyPriority, label: lbl("priority"), scope: scopeRuns},
	{keys: keyDelete, label: lbl("cancel"), scope: scopeRuns, commits: true},
	{keys: keyFilter, label: func(m model) string { return "filter: " + string(m.runFilter) }, scope: scopeRuns},
	{keys: keyScopeTog, label: func(m model) string { return "scope: " + scopeName(m.scopeRepo) }, scope: scopeRuns},
}

// footerFor renders a scope's "key label" hints — the navigating keys, plus one entry for the
// prefix. The footer carried a dozen per tab and had no room left for the movement keys; the menu
// lists the rest in context, which is what that buys back.
func (m model) footerFor(scope keyScope) string {
	var parts []string
	for _, b := range keymap {
		if b.scope == scope && !b.commits {
			parts = append(parts, b.keys+" "+b.label(m))
		}
	}
	if scope != scopeGlobal {
		parts = append(parts, keyMenuShown+" "+menuLabel(m, scope))
	}
	return strings.Join(parts, " · ")
}

// menuLabel names the prefix entry, counting what is actually on offer for the selected row: a bare
// "actions" on a row with none would send the reader into an empty box.
func menuLabel(m model, scope keyScope) string {
	n := 0
	for _, b := range keymap {
		if !b.commits || (b.scope != scope && b.scope != scopeGlobal) {
			continue
		}
		if b.when != nil && !b.when(m) {
			continue
		}
		n++
	}
	if n == 0 {
		return "actions (none here)"
	}
	return "actions"
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
	case 5:
		return scopeRuns
	case 6:
		return scopeMail
	default:
		return scopeRepos
	}
}
