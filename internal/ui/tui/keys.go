// package: tui / keys
// type:    ui (keymap — single source of truth)
// job:     the one place every hotkey is declared: its key, its help label, and the
// scope it belongs to. onKey dispatches on the key constants defined here,
// and the footers are generated from the keymap table — so a binding and
// its help can never drift apart.
// limits:  declaration + help rendering only; what each action does lives in onKey.
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Action-key constants: onKey switches on these and the keymap lists them, so a rebinding is one
// edit. Compound navigation rows (tab, pane, move) are display-only.
//
// Which letters commit is a UX judgement, case by case, not a rule to derive — an earlier version
// of this comment stated one ("opening a form or picker is navigating") and that is what went
// wrong (sd-6d0ff2). The one hard invariant: reaching the client on the bare keystroke, no form,
// no confirm, always commits (-> commitinvariant_test.go). Past that, a form/picker opener can
// land on either side; the call is recorded on the binding it was made for, not stated here.
// Committing sits behind the space prefix (keyMenu); M is still merge.
const (
	keyHelp      = "?" // list every hotkey — the current tab's, then global — conditions spelled out
	keyNew       = "N" // new task / new agent
	keyBrief     = "B" // hand a task to a planner to work up (brief)
	keyEdit      = "e" // edit the selection: task fields (tasks) / open the workspace in $EDITOR (agents, prs)
	keyOptions   = "O" // agents: an agent's options · tasks: reOpen a closed task — both open a form, harmless but uncommon, so both commit to reclaim footer space
	keyPriority  = "P" // set task priority — opens a picker; common enough that it stays direct
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
	keyTell      = "t" // tell an agent: PUSH, interrupts now / show a PR's task
	// Mail an agent: it waits to be read instead of interrupting, so the user picks the path at the
	// keyboard. Shares "i" with comment — both open a prompt, and neither commits on the keystroke.
	keyMail     = "i" // agents: mail (inbox) an agent — opens a prompt
	keyComment  = "i" // comment on a task — opens a prompt
	keyAttach   = "a" // attach to an agent's session (agents, tasks, prs)
	keyMerge    = "M" // merge a PR — commits on the keystroke, which is why it sits behind the prefix
	keyDelete   = "D" // delete an agent
	keyLint     = "L" // lint a PR
	keyVerify   = "V" // verify (materialize) a PR
	keyOpen     = "o" // open the row's worktree in a shell
	keyFilter   = "f" // cycle the tasks filter
	keyWhyNext  = "n" // what would be assigned next, and why nothing else would be (a view)
	keyScopeTog = "s" // toggle a tab's global↔repo scope
	keyRepo     = "p" // switch the active repo/project
	keyConfig   = "E" // edit the repo's config — opens a form, rarely used; commits to reclaim footer space
	keyColor    = "c" // pick a repo's colour — opens a picker
	keyRefresh  = "r" // refresh the board
	keySearch   = "/" // tasks: open a live search over the list — narrows as you type
	// Mail: narrow the list to the selected message's recipient — "who was told this?", the question
	// the tab is opened with. Its own letter because `a` attaches and only attaches, on every tab.
	keyMailWho = "w"
	keyMenu    = " " // the prefix: opens the menu of committing actions for the selected row
	// keyMenuShown is how the prefix reads in a footer: a bare space would render as a gap, and a
	// gap advertises nothing.
	keyMenuShown = "space"
	// esc, on a plain list: clear every narrowing at once. Unbound there until now, and "get me out
	// of this narrowed view" is what it already means over a modal or a prompt.
	keyClearFilters = "esc"
	keyDetail       = "§"     // toggle the detail pane
	keyQuit         = "q"     // quit
	keyEnter        = "enter" // not a single rune — named here so every use agrees on the string
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

// binding is one row of help: keys, label, scope, and whether it COMMITS. `when` narrows a
// binding to the rows it applies to; whenText names that condition for the help modal.
type binding struct {
	keys    string
	label   func(m model) string
	scope   keyScope
	commits bool
	when    func(m model) bool
	// whenText names `when` in words, for the help modal. Every binding with `when` set MUST carry
	// one (-> TestEveryWhenGatedBindingHasWhenText).
	whenText string
	// refOnly keeps a binding out of the footer (esc, enter — real dispatcher keys the footer rows
	// deliberately never carried) while still listing it in the "?" reference.
	refOnly bool
}

// lbl wraps a static label.
func lbl(s string) func(model) string { return func(model) string { return s } }

// keymap is the single source of truth for the actionable hotkeys shown in the
// footers. Order here is the order shown.
var keymap = []binding{
	// Global (first footer row): compound nav rows are display-only. "C-h/C-l", not "C-h/l" — the
	// trailing bare "l" would misread as the real, different binding tasks: expand a fold uses.
	{keys: keyHelp, label: lbl("help"), scope: scopeGlobal}, // leads the row: kept longest if it sheds
	{keys: "⇥/⇧⇥", label: lbl("tab"), scope: scopeGlobal},
	// 1-N jumps straight to a tab by its header number (tui.go); out of range is inert, never a
	// jump to whatever the last tab happens to be (-> onKey's digit case).
	{keys: fmt.Sprintf("1-%d", len(tuiSections)), label: lbl("jump"), scope: scopeGlobal},
	// C-l steps forward through this tab's panes past the list (its raw content, then its actionable
	// items if any), wrapping back to the list; C-h always returns straight to it.
	{keys: "C-h/C-l", label: lbl("pane"), scope: scopeGlobal},
	// j/k move the row cursor by default; once a pane is focused (C-l) they move within IT instead —
	// scrolling its content one line at a time, or stepping its actionable items (-> onKey).
	{keys: "j/k/g/G", label: lbl("move/top/bot"), scope: scopeGlobal},
	// "page", not "scroll detail": ctrl+d/ctrl+u half-page whichever pane has focus — the list, or
	// whatever C-l last focused — so one fixed label would be wrong half the time.
	{keys: "C-d/C-u", label: lbl("page"), scope: scopeGlobal},
	{keys: "y/Y", label: lbl("yank/all"), scope: scopeGlobal},
	{keys: keyDetail, label: lbl("detail"), scope: scopeGlobal},
	{keys: keyRepo, label: lbl("repo"), scope: scopeGlobal},
	{keys: keyConfig, label: lbl("config"), scope: scopeGlobal, commits: true},
	{keys: keyRefresh, label: lbl("refresh"), scope: scopeGlobal},
	// esc is a real dispatcher key the footer never carried (refOnly keeps it off, but not off
	// "?"). enter has no global row here: its meaning is per-tab, declared per scope below.
	{keys: keyClearFilters, label: lbl("clear filters"), scope: scopeGlobal, refOnly: true},
	{keys: keyQuit, label: lbl("quit"), scope: scopeGlobal},

	// Tasks: each scope's rows are grouped and ordered look-first, so the footer reads left to
	// right from the harmless to the decisive.
	{keys: keyNew, label: lbl("new"), scope: scopeTasks, commits: true}, // was direct; now matches Chat's N (sd-5e3032)
	{keys: "h/l", label: lbl("fold"), scope: scopeTasks},
	{keys: keyBrief, label: lbl("brief a planner"), scope: scopeTasks},
	{keys: keyComment, label: lbl("comment"), scope: scopeTasks},
	{keys: keyAttach, label: lbl("attach"), scope: scopeTasks},
	{keys: keyEdit, label: lbl("edit"), scope: scopeTasks},
	{keys: keyPriority, label: lbl("priority"), scope: scopeTasks},
	{keys: keySearch, label: lbl("search"), scope: scopeTasks},
	{keys: keyUnassign, label: lbl("unassign"), scope: scopeTasks, commits: true, when: taskHeld, whenText: "while an agent holds it"},
	{keys: keyClose, label: lbl("close"), scope: scopeTasks, commits: true, when: taskOpen, whenText: "while it's open"},
	{keys: keyOptions, label: lbl("reopen"), scope: scopeTasks, commits: true, when: model.taskReopenable, whenText: "while it's closed"},
	{keys: keyDelete, label: lbl("scrap"), scope: scopeTasks, commits: true},
	{keys: keyApprove, label: lbl("approve"), scope: scopeTasks, commits: true, when: taskAwaitsVerdict, whenText: "while awaiting your verdict"},
	{keys: keyReject, label: lbl("reject"), scope: scopeTasks, commits: true, when: taskAwaitsVerdict, whenText: "while awaiting your verdict"},
	{keys: keyWhyNext, label: lbl("why next"), scope: scopeTasks},
	{keys: keyFilter, label: func(m model) string { return "filter: " + string(m.filter) }, scope: scopeTasks},
	{keys: keyEnter, label: lbl("full screen"), scope: scopeTasks, refOnly: true},
	// Least decisive last on purpose: at a narrow width this is what truncation should eat first,
	// not the tab's own actions above it (-> idsNeedingUser; Runs/Repos/Meeting have no such notion).
	{keys: "[/]", label: lbl("needs-you"), scope: scopeTasks},

	// Agents.
	{keys: keyNew, label: lbl("new"), scope: scopeAgents, commits: true}, // same call as Tasks' N (sd-5e3032)
	// The following all silently no-op on an orphan container (selAgent finds nothing to act on),
	// so each needs agentSelected to say so.
	{keys: keyTell, label: lbl("tell: push now"), scope: scopeAgents, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyMail, label: lbl("mail: waits"), scope: scopeAgents, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyAttach, label: lbl("attach"), scope: scopeAgents, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyEdit, label: lbl("editor"), scope: scopeAgents, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyOpen, label: lbl("open"), scope: scopeAgents},
	{keys: keyStartS, label: lbl("start/stop"), scope: scopeAgents, commits: true, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyOptions, label: lbl("options"), scope: scopeAgents, commits: true, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyStats, label: lbl("stats"), scope: scopeAgents},
	{keys: keyMilestone, label: lbl("milestone PR"), scope: scopeAgents, commits: true, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyRebuild, label: lbl("rebuild image"), scope: scopeAgents, commits: true, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	// R = reBase (onto the reference branch)
	{keys: keyReject, label: lbl("rebase"), scope: scopeAgents, commits: true, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	// The label tracks the selection, since the key toggles and "retire" on an already-retired
	// agent reads as a no-op the user would not press.
	{keys: keyRetire, label: func(m model) string {
		if a, ok := m.selAgent(); ok && a.Retired {
			return "unretire"
		}
		return "retire"
	}, scope: scopeAgents, commits: true, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyClearCtx, label: func(m model) string {
		if a, ok := m.selAgent(); ok && a.ClearArmed {
			return "cancel clear"
		}
		return "clear context"
	}, scope: scopeAgents, commits: true, when: agentSelected, whenText: "while a roster agent, not an orphan"},
	{keys: keyDelete, label: lbl("delete"), scope: scopeAgents, commits: true},
	{keys: keyScopeTog, label: func(m model) string { return "scope: " + scopeName(m.scopeRepo, m) }, scope: scopeAgents},
	{keys: keyEnter, label: lbl("full screen"), scope: scopeAgents, refOnly: true},
	{keys: "[/]", label: lbl("needs-you"), scope: scopeAgents},

	// PRs: look (verify/editor/open/lint), then the verdicts, then merge.
	{keys: keyVerify, label: lbl("verify"), scope: scopePRs, commits: true},
	{keys: keyEdit, label: lbl("editor"), scope: scopePRs},
	{keys: keyOpen, label: lbl("open"), scope: scopePRs},
	{keys: keyAttach, label: lbl("attach"), scope: scopePRs},
	{keys: keyTell, label: lbl("show linked task"), scope: scopePRs, when: prShowsLinkedTask, whenText: "while the PR has a linked task"},
	{keys: keyLint, label: lbl("lint"), scope: scopePRs, commits: true},
	{keys: keyApprove, label: lbl("approve"), scope: scopePRs, commits: true, when: prDecidable, whenText: "while the PR is still open"},
	{keys: keyReject, label: lbl("reject"), scope: scopePRs, commits: true},
	{keys: keyReview, label: lbl("agent-review"), scope: scopePRs, commits: true},
	{keys: keyMerge, label: lbl("merge"), scope: scopePRs, commits: true, when: model.selPRApproved, whenText: "once it's approved"},
	{keys: keyDelete, label: lbl("scrap"), scope: scopePRs, commits: true},
	{keys: keyWhyNext, label: lbl("why no review"), scope: scopePRs},
	{keys: keyFilter, label: func(m model) string { return "filter: " + string(m.prFilter) }, scope: scopePRs},
	{keys: keyScopeTog, label: func(m model) string { return "scope: " + scopeName(m.scopeRepo, m) }, scope: scopePRs},
	{keys: keyEnter, label: lbl("full screen"), scope: scopePRs, refOnly: true},
	{keys: "[/]", label: lbl("needs-you"), scope: scopePRs},

	// Repos. config is not redeclared here: scopeGlobal+commits already reaches every tab's menu.
	{keys: keyEnter, label: lbl("switch"), scope: scopeRepos},
	{keys: keyColor, label: lbl("colour"), scope: scopeRepos},
	{keys: keyDelete, label: lbl("forget"), scope: scopeRepos, commits: true},

	// Mail: look only — the mailbox is the agent's to read, and the user's part is finding a message.
	{keys: keyAttach, label: lbl("attach"), scope: scopeMail, when: mailAttachable, whenText: "while either party is a live agent"},
	{keys: keyMail, label: lbl("reply"), scope: scopeMail},
	{keys: keyMailWho, label: func(m model) string { return "who: " + mailWhoLabel(m.mailAgent) }, scope: scopeMail},
	{keys: keyFilter, label: func(m model) string { return "filter: " + string(m.mailFilter) }, scope: scopeMail},
	{keys: keyScopeTog, label: func(m model) string { return "scope: " + scopeName(m.scopeRepo, m) }, scope: scopeMail},
	{keys: keyEnter, label: lbl("full screen"), scope: scopeMail, refOnly: true},
	{keys: "[/]", label: lbl("needs-you"), scope: scopeMail},

	// Chat.
	{keys: keyEnter, label: lbl("compose"), scope: scopeChat},
	{keys: keyApprove, label: lbl("add member"), scope: scopeChat, commits: true},
	{keys: keyReject, label: lbl("remove member"), scope: scopeChat, commits: true},
	{keys: keyNew, label: lbl("new meeting"), scope: scopeChat, commits: true},
	{keys: keyCloseMeet, label: lbl("close meeting"), scope: scopeChat, commits: true},

	// Runs.
	{keys: keyNew, label: lbl("queue a run"), scope: scopeRuns, commits: true}, // opens a prompt (sd-5e3032)
	{keys: keyPriority, label: lbl("priority"), scope: scopeRuns},
	{keys: keyDelete, label: lbl("cancel"), scope: scopeRuns, commits: true},
	{keys: keyFilter, label: func(m model) string { return "filter: " + string(m.runFilter) }, scope: scopeRuns},
	{keys: keyScopeTog, label: func(m model) string { return "scope: " + scopeName(m.scopeRepo, m) }, scope: scopeRuns},
	{keys: keyEnter, label: lbl("full screen"), scope: scopeRuns, refOnly: true},
}

// footerFor renders a scope's plain "key label" hints — no prefix; that is global now (-> globalFooter).
func (m model) footerFor(scope keyScope) string {
	var parts []string
	for _, b := range keymap {
		if b.scope != scope || b.commits || b.refOnly {
			continue
		}
		if b.when != nil && !b.when(m) {
			continue
		}
		parts = append(parts, b.keys+" "+b.label(m))
	}
	return strings.Join(parts, " · ")
}

// globalFooter is the first footer row: "?" leading, the prefix trailing, both pinned since each
// names where the rest still reads in full — only the navigation between them sheds.
func (m model) globalFooter(width int) string {
	var lead string
	var middle []globalEntry
	for _, b := range keymap {
		if b.scope != scopeGlobal || b.commits || b.refOnly {
			continue
		}
		if b.when != nil && !b.when(m) {
			continue
		}
		if b.keys == keyHelp {
			lead = b.keys + " " + b.label(m)
			continue
		}
		for _, p := range focusSplit(b, m, m.focus) {
			middle = append(middle, globalEntry{label: p.label, rank: b.label(m), text: p.keys + " " + p.label})
		}
	}
	trail := keyMenuShown + " " + menuLabel(m, tabScope(m.tab))
	return shedMiddle(lead, middle, trail, width)
}

// globalEntry is one global-row entry: text is shown, label its display identity, rank the
// parent binding's own label — so a focus-remapped display label still sheds by its parent's rank.
type globalEntry struct{ label, rank, text string }

// globalShedOrder ranks the global row's entries least useful (shed first) to most useful (kept
// longest) — sd-5e3032's own instruction, a usefulness order distinct from keymap's own reading
// order.
var globalShedOrder = []string{
	"jump", "pane", "repo", "yank/all", "detail", "refresh", "page",
	"tab", "move/top/bot", "quit",
}

// shedPriority is an entry's position in globalShedOrder — lower sheds first. An entry not listed
// there sorts last (shed last, not skipped) — shedMiddle's loop still reaches it eventually.
func shedPriority(label string) int {
	for i, l := range globalShedOrder {
		if l == label {
			return i
		}
	}
	return len(globalShedOrder)
}

// shedMiddle joins lead, middle and trail with " · ", dropping whole entries — by shedPriority,
// least useful first, never lead or trail — until the row fits width. Survivors keep keymap's own
// declaration order; only which ones survive changes.
func shedMiddle(lead string, middle []globalEntry, trail string, width int) string {
	join := func(keep []globalEntry, shed bool) string {
		parts := []string{lead}
		for _, e := range keep {
			parts = append(parts, e.text)
		}
		if shed {
			parts = append(parts, "…")
		}
		return strings.Join(append(parts, trail), " · ")
	}
	if line := join(middle, false); ansi.StringWidth(line) <= width {
		return line
	}
	byPriority := append([]globalEntry(nil), middle...)
	sort.SliceStable(byPriority, func(i, j int) bool {
		return shedPriority(byPriority[i].rank) < shedPriority(byPriority[j].rank)
	})
	dropped := map[string]bool{}
	for _, e := range byPriority {
		dropped[e.label] = true
		var keep []globalEntry
		for _, e := range middle {
			if !dropped[e.label] {
				keep = append(keep, e)
			}
		}
		if line := join(keep, true); ansi.StringWidth(line) <= width {
			return line
		}
	}
	// Reachable only once every middle entry is shed and lead+trail alone still overflow width —
	// realistically never, since a terminal narrow enough for that cannot show a usable footer at
	// all. ansi.Truncate supplies its own "…" here, which is why join's shed marker is off.
	return ansi.Truncate(join(nil, false), width, "…")
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

// helpLines is "?"'s reference: the current tab first, GLOBAL after, unfiltered by `when`. Focus
// relabels only its own override table's keys, never a whole section.
func (m model) helpLines() []string {
	global := map[string]bool{}
	for _, b := range keymap {
		if b.scope == scopeGlobal {
			global[b.keys+"\x00"+b.label(m)] = true
		}
	}
	lines := []string{tuiSections[m.tab].Title}
	lines = append(lines, helpRows(m, tabScope(m.tab), global, m.focus)...)
	lines = append(lines, "", "GLOBAL")
	lines = append(lines, helpRows(m, scopeGlobal, map[string]bool{}, m.focus)...)
	return lines
}

// focusOverrides decomposes a focus state's table to one entry per literal key, splitting a
// compound row (e.g. "j/k/g/G") into relabelled and ordinary parts. focusList has none.
func focusOverrides(focus focusPane) map[string]string {
	var table []focusKey
	switch focus {
	case focusItems:
		table = rightFocusKeys
	case focusDetail:
		table = detailFocusKeys
	default:
		return nil
	}
	out := map[string]string{}
	for _, r := range table {
		for _, k := range strings.Split(r.keys, "/") {
			out[k] = r.label
		}
	}
	return out
}

// helpRows renders every binding in scope as one reference line, or two if focus splits it between
// a remapped meaning and an ordinary one (-> focusRows). exclude skips a binding declared globally too.
func helpRows(m model, scope keyScope, exclude map[string]bool, focus focusPane) []string {
	var rows []string
	for _, b := range keymap {
		if b.scope != scope {
			continue
		}
		id := b.keys + "\x00" + b.label(m)
		if exclude[id] {
			continue
		}
		rows = append(rows, focusRows(b, m, focus)...)
	}
	return rows
}

// focusPart is one piece of a binding split by focus: overridden marks the relabelled half (a
// different action, so formatRow's commits/whenText do not apply), false for the half left with
// the binding's own meaning.
type focusPart struct {
	keys       string
	label      string
	overridden bool
}

// focusSplit is the one place a binding's keys are split by focus — used by both globalFooter and
// the reference, so the two cannot disagree about what a key does in any of the three states.
func focusSplit(b binding, m model, focus focusPane) []focusPart {
	overrides := focusOverrides(focus)
	if overrides == nil {
		return []focusPart{{b.keys, b.label(m), false}}
	}
	byLabel := map[string][]string{}
	var order, left []string
	for _, k := range strings.Split(b.keys, "/") {
		lbl, ok := overrides[k]
		if !ok {
			left = append(left, k)
			continue
		}
		if _, seen := byLabel[lbl]; !seen {
			order = append(order, lbl)
		}
		byLabel[lbl] = append(byLabel[lbl], k)
	}
	if len(order) == 0 {
		return []focusPart{{b.keys, b.label(m), false}}
	}
	parts := make([]focusPart, 0, len(order)+1)
	for _, lbl := range order {
		parts = append(parts, focusPart{strings.Join(byLabel[lbl], "/"), lbl, true})
	}
	if len(left) > 0 {
		parts = append(parts, focusPart{strings.Join(left, "/"), b.label(m), false})
	}
	return parts
}

// focusRows renders one binding's focusSplit parts as reference lines: formatRow for an
// unoverridden part (keeps commits/whenText), the bare label for an overridden one — safe only
// while TestFocusOverriddenKeysNeverCommitOrCarryAWhen holds.
func focusRows(b binding, m model, focus focusPane) []string {
	var rows []string
	for _, p := range focusSplit(b, m, focus) {
		if p.overridden {
			rows = append(rows, p.keys+"  "+p.label)
		} else {
			rows = append(rows, formatRow(p.keys, b, m))
		}
	}
	return rows
}

// formatRow is one binding's reference line at the given key text: the prefix folded in
// ("space X") and any `when` condition named after it.
func formatRow(keys string, b binding, m model) string {
	if b.commits {
		keys = keyMenuShown + " " + keys
	}
	row := keys + "  " + b.label(m)
	if b.whenText != "" {
		row += " (" + b.whenText + ")"
	}
	return row
}
