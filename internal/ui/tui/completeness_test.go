package tui

import (
	"reflect"
	"testing"
)

// dispatchedKeys is every literal key onKey actually dispatches, per tab (-1 = every tab,
// regardless, checked once against scopeGlobal alone). Kept as an explicit, commented list rather
// than derived from onkey.go, so a new case there is a deliberate edit here too — the same
// argument internal/arch/context_test.go makes for its own allowlist.
//
// Left out on purpose: keys onkey.go dispatches through a display-only compound row rather than a
// literal single key match — tab/shift+tab ("⇥/⇧⇥"), ctrl+h/ctrl+l ("C-h/C-l"), ctrl+d/ctrl+u
// ("C-d/C-u"), and the digit tab-jumps ("1-N") — plus "down"/"up", bare aliases of j/k already
// covered by them. Also left out: keys onKey reaches only through a focused cross-reference item
// (view/mail/resume/path/url/mailbody under "enter"'s rightFocus branch) — those describe the
// item in the pane, not a fixed per-tab meaning, and already have their own hint line
// (contextFooter's rightFocus case). `]`/`[` are dispatched everywhere (moveToNeedingUser decides
// per tab whether they do anything) but listed only on the four tabs that advertise them.
var dispatchedKeys = map[int][]string{
	-1: {keyHelp, keyQuit, "y", "Y", "j", "k", "g", "G",
		keyClearFilters, keyDetail, keyRefresh, keyRepo, keyConfig},
	0: {keyFilter, keySearch, "h", "l", keyAttach, keyNew, keyEdit, keyComment, keyBrief,
		keyOptions, keyDelete, keyPriority, keyUnassign, keyWhyNext, keyClose, keyReject,
		keyApprove, keyEnter, "]", "["},
	1: {keyNew, keyTell, keyComment, keyAttach, keyEdit, keyOpen, keyStartS, keyOptions, keyStats,
		keyBrief, keyReject, keyRetire, keyClose, keyDelete, keyScopeTog, keyMerge, keyEnter, "]", "["},
	2: {keyVerify, keyEdit, keyOpen, keyAttach, keyTell, keyLint, keyApprove, keyReject, keyReview,
		keyMerge, keyDelete, keyFilter, keyScopeTog, keyWhyNext, keyEnter, "]", "["},
	3: {keyDelete, keyColor, keyEnter},
	4: {keyNew, keyClose, keyReject, keyApprove, keyEnter},
	5: {keyNew, keyPriority, keyDelete, keyFilter, keyScopeTog, keyEnter},
	6: {keyMailWho, keyComment, keyAttach, keyFilter, keyScopeTog, keyEnter, "]", "["},
}

// keymapHas reports whether some binding in scope or scopeGlobal dispatches on key k — the same
// question dispatchKeyParts answers for the conflict guard, reused here for completeness instead.
func keymapHas(scope keyScope, k string) bool {
	for _, b := range keymap {
		if b.scope != scope && b.scope != scopeGlobal {
			continue
		}
		for _, part := range dispatchKeyParts(b.keys) {
			if part == k {
				return true
			}
		}
	}
	return false
}

// TestEveryDispatchedKeyHasAKeymapRow is the completeness half "?" needs and dispatchKeyParts
// never gave it: that guard only stops two rows from claiming the same key, it says nothing about
// a real onKey case with no row at all — which is exactly how keyTell on the PRs tab (t: show
// linked task) went undiscoverable. One row per case here closes it.
func TestEveryDispatchedKeyHasAKeymapRow(t *testing.T) {
	for tab, keys := range dispatchedKeys {
		scope := scopeGlobal
		if tab >= 0 {
			scope = tabScope(tab)
		}
		for _, k := range keys {
			if !keymapHas(scope, k) {
				t.Errorf("tab %d: %q is dispatched by onKey but has no keymap row in scope %d or global", tab, k, scope)
			}
		}
	}
}

// TestEveryKeymapRowHasADispatchCase is dispatchedKeys' other direction: a row promising a key
// that onKey never actually acts on — the shape of "s" once being advertised on Runs and Mail but
// dispatched on neither — is the class this catches, not just one instance of it.
func TestEveryKeymapRowHasADispatchCase(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	scopeTab := map[keyScope]int{scopeGlobal: -1}
	for tab := range tuiSections {
		scopeTab[tabScope(tab)] = tab
	}
	for _, b := range keymap {
		tab, ok := scopeTab[b.scope]
		if !ok {
			t.Fatalf("scope %d has no tab in tuiSections", b.scope)
		}
		for _, part := range dispatchKeyParts(b.keys) {
			listed := false
			for _, k := range dispatchedKeys[tab] {
				if k == part {
					listed = true
				}
			}
			if !listed {
				t.Errorf("tab %d: keymap row %q (%q) is not in dispatchedKeys — does onKey actually act on it?",
					tab, part, b.label(m))
			}
		}
	}
}

// gateKey identifies one keymap row by the tab it shows on and its literal key — a dispatch gate
// is recorded per row, not per key alone, since the same letter can gate on different things (or
// nothing) on different tabs.
type gateKey struct {
	tab  int
	keys string
}

// dispatchGates names, for every row whose keymap `when` narrows it, the predicate that actually
// enforces that condition — hand-maintained, like dispatchedKeys, so a hidden gate (a `when` of
// nil where onKey's real case is conditional) cannot silently drift from the row meant to describe
// it. Most entries are checked directly inside onKey's own case (or a helper it calls straight
// into, e.g. agentStartStop). {0,keyUnassign}, {0,keyClose} and {2,keyApprove} commit with no such
// check at all: their case runs unconditionally once the menu has already gated the keystroke, so
// the predicate recorded here is what the MENU enforces, not a second check inside onKey.
//
// {1,keyEdit} is an approximation: onKey's real condition is agentWorkspacePath returning
// non-empty, which agentSelected does not fully capture (a real agent with no resolvable workspace
// path would still read as selected) — the common, user-visible case is an orphan container, so
// that is what is recorded.
var dispatchGates = map[gateKey]func(model) bool{
	{0, keyUnassign}:  taskHeld,
	{0, keyClose}:     taskOpen,
	{0, keyApprove}:   model.taskGated,
	{0, keyReject}:    model.taskGated,
	{0, keyOptions}:   model.taskReopenable,
	{1, keyTell}:      agentSelected,
	{1, keyMail}:      agentSelected,
	{1, keyAttach}:    agentSelected,
	{1, keyEdit}:      agentSelected,
	{1, keyStartS}:    agentSelected,
	{1, keyOptions}:   agentSelected,
	{1, keyMilestone}: agentSelected,
	{1, keyRebuild}:   agentSelected,
	{1, keyReject}:    agentSelected,
	{1, keyRetire}:    agentSelected,
	{1, keyClearCtx}:  agentSelected,
	{2, keyApprove}:   prDecidable,
	{2, keyMerge}:     model.selPRApproved,
	{2, keyTell}:      prShowsLinkedTask,
	{6, keyAttach}:    mailAttachable,
}

// TestDispatchGatesMatchTheirKeymapRow catches a row whose `when` disagrees with what onKey
// actually gates it on — including `when: nil` where onKey's real case is conditional, the shape
// TestEveryWhenGatedBindingHasWhenText cannot see (nothing to check when there is no `when` at
// all) and the other two completeness guards cannot see either (the key IS dispatched, and the row
// IS listed for it — the condition is one level deeper, in what onKey actually needs). It does not
// cover whether a row is selected at all: a predicate here can pass on an empty list the same way
// onKey's own case does, and neither this guard nor onKey is asked to notice that.
func TestDispatchGatesMatchTheirKeymapRow(t *testing.T) {
	scopeTab := map[keyScope]int{}
	for tab := range tuiSections {
		scopeTab[tabScope(tab)] = tab
	}
	for _, b := range keymap {
		if b.scope == scopeGlobal || b.refOnly {
			continue
		}
		want, gated := dispatchGates[gateKey{scopeTab[b.scope], b.keys}]
		switch {
		case gated && b.when == nil:
			t.Errorf("tab %d %q: onKey gates this on a condition, but the keymap row has no `when`",
				scopeTab[b.scope], b.keys)
		case gated && reflect.ValueOf(b.when).Pointer() != reflect.ValueOf(want).Pointer():
			t.Errorf("tab %d %q: keymap `when` does not match the recorded dispatch gate", scopeTab[b.scope], b.keys)
		case !gated && b.when != nil:
			t.Errorf("tab %d %q: keymap row has a `when`, but dispatchGates has no recorded gate for it — "+
				"add one so this table stays exhaustive", scopeTab[b.scope], b.keys)
		}
	}
}
