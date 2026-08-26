package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestHelpKeyOpensAReferenceModal: "?" is direct — it is a view, so it stays outside the prefix —
// and opens the same scrollable modal every other detail view uses.
func TestHelpKeyOpensAReferenceModal(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.w, m.h = 100, 30
	m.tab = 0
	m.onKey(keyHelp)
	if !m.modal {
		t.Fatal("? should open the help modal")
	}
	if m.modalOverrideTitle != "Help" {
		t.Errorf("the modal title should just say Help, got %q", m.modalOverrideTitle)
	}
	got := strings.Join(m.modalLines(), "\n")
	if !strings.Contains(got, "GLOBAL") || !strings.Contains(got, "Tasks") {
		t.Errorf("the modal should show both the global bindings and the current tab's:\n%s", got)
	}
}

// TestHelpModalShowsBindingsTheFooterHides is the addendum's whole point: the footer answers "what
// can I press now" and hides what `when` refuses, but the modal answers "what exists on this tab"
// — a binding hidden most of the time (U unassign, only while an agent holds the task) must still
// be findable, with its condition spelled out, or a user has no moment at which to discover it.
func TestHelpModalShowsBindingsTheFooterHides(t *testing.T) {
	m := tasksTabWith(api.Task{ID: "td-1", Title: "unheld", Status: "open"})
	m.w, m.h = 100, 30

	footer := m.contextFooter()
	if strings.Contains(footer, "unassign") {
		t.Fatalf("precondition: the footer should hide unassign on an unheld task:\n%s", footer)
	}

	m.onKey(keyHelp)
	got := strings.Join(m.modalLines(), "\n")
	if !strings.Contains(got, "unassign") {
		t.Errorf("the help modal must list unassign even though nothing holds td-1:\n%s", got)
	}
	if !strings.Contains(got, "while an agent holds it") {
		t.Errorf("the help modal must spell out unassign's condition:\n%s", got)
	}
}

// TestHelpModalMarksCommittingBindingsWithTheirPrefix: a binding that commits reaches the client
// through space first, and the reference must say so rather than imply a bare keystroke works.
func TestHelpModalMarksCommittingBindingsWithTheirPrefix(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 0 // Tasks: N now commits (sd-5e3032)
	m.onKey(keyHelp)
	got := strings.Join(m.modalLines(), "\n")
	if !strings.Contains(got, keyMenuShown+" "+keyNew+"  new") {
		t.Errorf("a committing binding should read \"space N\", not bare, in the help modal:\n%s", got)
	}
}

// TestHelpModalShowsTheCurrentTabWithinTheFold: at 80x24, GLOBAL's 14 rows plus header would push
// all but one of Tasks' own bindings — U/A/R among them, exactly what sd-5e3032's addendum was
// written to make discoverable — below modalContentHeight(24)'s visible window. The tab's own
// section must lead, so its bindings are what a reader sees first.
//
// The Tasks and Agents sections are each exactly 18 lines at this size — modalContentHeight(24)'s
// entire budget, none spare. This only pins U/A/R; it will not notice one more binding on either
// tab pushing something below the fold. Adding a row there should come with a width/height check,
// not just a green run of this test.
func TestHelpModalShowsTheCurrentTabWithinTheFold(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.w, m.h = 80, 24
	m.tab = 0 // Tasks
	m.onKey(keyHelp)
	lines := m.modalLines()
	if lines[0] != tuiSections[0].Title {
		t.Fatalf("the current tab's section should lead the reference, got %q", lines[0])
	}
	visible := strings.Join(lines[:min(len(lines), modalContentHeight(m.h))], "\n")
	for _, want := range []string{keyUnassign + "  unassign", keyApprove + "  approve", keyReject + "  reject"} {
		if !strings.Contains(visible, want) {
			t.Errorf("%q should be visible within the fold at 80x24, got:\n%s", want, visible)
		}
	}
}

// TestHelpModalNeverListsTheSameBindingTwice: config used to be declared both globally and on
// Repos — genuine redundancy, since it is scopeGlobal and commits, so menuOffers (which DOES admit
// global committing bindings on every tab) offered it twice and menuFooter rendered both with no
// dedup ("E config · E config" on Repos). The fix was deleting the redundant Repos row, not
// masking it in the reference alone; helpRows' exclude-by-global is a defensive guard past that,
// not the fix itself.
func TestHelpModalNeverListsTheSameBindingTwice(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 3 // Repos
	lines := m.helpLines()
	want := keyMenuShown + " " + keyConfig + "  config"
	globalAt := -1
	configLine := -1
	for i, l := range lines {
		if l == "GLOBAL" {
			globalAt = i
		}
		if l == want {
			if configLine >= 0 {
				t.Fatalf("config appears more than once: lines %d and %d:\n%s", configLine, i, strings.Join(lines, "\n"))
			}
			configLine = i
		}
	}
	if configLine < 0 {
		t.Fatalf("config should be listed in the Repos reference:\n%s", strings.Join(lines, "\n"))
	}
	if configLine < globalAt {
		t.Errorf("config should be attributed to GLOBAL (line %d), not the Repos section (found at line %d)", globalAt, configLine)
	}
}

// TestReposMenuOffersConfigOnce is the actual bug the redundant row caused: menuOffers admits
// global committing bindings on every tab (component_menu.go's own comment says so), so a
// Repos-scoped config row doubled it there, not just in the reference.
func TestReposMenuOffersConfigOnce(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 3 // Repos
	n := 0
	for _, b := range m.menuOffers() {
		if b.keys == keyConfig {
			n++
		}
	}
	if n != 1 {
		t.Errorf("config should be offered exactly once on the Repos menu, got %d", n)
	}
}

// TestGlobalFooterAgreesWithTheLocalRowWhileFocused: globalFooter used to render scopeGlobal rows
// verbatim under focus, so the footer's own two rows disagreed with each other on screen — "j/k/g/G
// move/top/bot" (global) beside "j/k item" (local), while onkey.go's j/k/g/y cases all branch on
// rightFocus and do something else entirely. Both rows must generate through the same focusSplit.
func TestGlobalFooterAgreesWithTheLocalRowWhileFocused(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 0
	m.state = api.BoardState{Tasks: []api.Task{{ID: "td-1", Status: "open"}}}
	m.reclamp()
	m.focus = focusItems
	global := m.globalFooter(500)
	local := m.contextFooter()
	for _, want := range []string{"j/k item", "g goto", "G bottom", "y copy"} {
		if !strings.Contains(global, want) {
			t.Errorf("the global row should read %q under focus, got %q", want, global)
		}
		if !strings.Contains(local, want) {
			t.Fatalf("precondition: the local row should also read %q, got %q", want, local)
		}
	}
	if strings.Contains(global, "j/k/g/G") {
		t.Errorf("the global row should not still read the un-focused j/k/g/G row, got %q", global)
	}
}

// TestHelpModalMatchesTheFooterWhileFocused: while the detail/meta column has focus, j/k/enter/g/y
// mean what rightFocusKeys says, not the tab's ordinary bindings — and contextFooter already says
// so on its own row, so "?" must agree with it rather than describing a different world.
func TestHelpModalMatchesTheFooterWhileFocused(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 0
	m.state = api.BoardState{Tasks: []api.Task{{ID: "td-1", Title: "a task", Status: "open"}}}
	m.reclamp()
	m.focus = focusItems
	footer := m.contextFooter()
	m.onKey(keyHelp)
	got := strings.Join(m.modalLines(), "\n")
	for _, r := range rightFocusKeys {
		want := r.keys + "  " + r.label
		if !strings.Contains(got, want) {
			t.Errorf("the reference should list %q while focused, got:\n%s", want, got)
		}
		if !strings.Contains(footer, r.keys+" "+r.label) {
			t.Fatalf("precondition: contextFooter should also name %q", r.keys+" "+r.label)
		}
	}
	// Every non-remapped binding must survive focus — unassign among them, one U/A/R the addendum
	// was written to surface. Those are `when`-gated, not focus-gated (onkey.go checks neither
	// m.focus != focusItems), so they must still read even though this task holds no agent.
	if !strings.Contains(got, "unassign") {
		t.Errorf("the reference should still list Tasks' un-focused bindings while focused, got:\n%s", got)
	}
	if strings.Contains(got, "j/k/g/G  move/top/bot") || strings.Contains(got, "G  move/top/bot") {
		t.Errorf("the global row's j/k/g/G should be relabelled under focus, not left reading their un-focused meaning:\n%s", got)
	}
}

// TestFocusOverriddenKeysNeverCommitOrCarryAWhen: focusRows' relabelled branch renders the focused
// meaning directly, not through formatRow — deliberately, since the focused action (item, details,
// goto, copy, scroll, top/bot) is a different action from whatever the un-focused binding does, and
// inheriting its `commits`/`whenText` would describe that wrong action instead. That is only safe
// as long as no binding whose keys intersect EITHER focus state's overrides actually commits or
// carries a `when` — this is the guard for that invariant, since nothing else would notice it slip.
func TestFocusOverriddenKeysNeverCommitOrCarryAWhen(t *testing.T) {
	for _, focus := range []focusPane{focusItems, focusDetail} {
		overrides := focusOverrides(focus)
		for _, b := range keymap {
			for _, k := range strings.Split(b.keys, "/") {
				if _, overridden := overrides[k]; !overridden {
					continue
				}
				if b.commits {
					t.Errorf("%q (%s) intersects focus %v's overrides and commits — the focused label would hide that", b.keys, b.label(newModel(nil, nil, "/r/one")), focus)
				}
				if b.whenText != "" {
					t.Errorf("%q (%s) intersects focus %v's overrides and carries a `when` — the focused label would hide that", b.keys, b.label(newModel(nil, nil, "/r/one")), focus)
				}
			}
		}
	}
}

// TestFooterAndHelpAgreeInEveryFocusState pins the invariant rightFocusKeys/detailFocusKeys exist
// to hold: contextFooter's hint and "?"'s reference must never describe two different worlds for
// the same focus state. Review round 5 found focusDetail had drifted from it — the footer knew
// j/k scroll and g/G jump top/bottom there, but helpLines still read the un-focused j/k/g/G row.
func TestFooterAndHelpAgreeInEveryFocusState(t *testing.T) {
	for _, tab := range []int{0, 2} { // a single-region tab and PRs' two-region one
		for _, tc := range []struct {
			focus focusPane
			table []focusKey
		}{
			{focusItems, rightFocusKeys},
			{focusDetail, detailFocusKeys},
		} {
			m := newModel(nil, nil, "/r/one")
			m.tab = tab
			m.focus = tc.focus
			footer := m.contextFooter()
			help := strings.Join(m.helpLines(), "\n")
			for _, r := range tc.table {
				if !strings.Contains(footer, r.keys+" "+r.label) {
					t.Errorf("tab %d focus %v: footer is missing %q %q, got %q", tab, tc.focus, r.keys, r.label, footer)
				}
				if !strings.Contains(help, r.keys+"  "+r.label) {
					t.Errorf("tab %d focus %v: help reference is missing %q  %q, got:\n%s", tab, tc.focus, r.keys, r.label, help)
				}
			}
		}
	}
}

// TestFoldWorksUnderFocus: folding the row under the cursor needs no particular column focused,
// so onKey no longer gates h/l on focus — the reference has no way to mark a binding
// "disabled here" (only "remapped"), so this keeps the two agreeing without new machinery.
func TestFoldWorksUnderFocus(t *testing.T) {
	m := tasksTabWith(api.Task{ID: "td-1", Title: "parent", Status: "open"},
		api.Task{ID: "td-1.1", Title: "child", Status: "open", ParentID: "td-1"})
	m.focus = focusItems
	m.onKey("h")
	if !m.collapsed["td-1"] {
		t.Error("h should collapse the row under the cursor even while the detail column has focus")
	}
	m.onKey("l")
	if m.collapsed["td-1"] {
		t.Error("l should expand the row under the cursor even while the detail column has focus")
	}
}

// TestHelpModalStillWorksAlongsideTheMenuWhileFocused: focused, on Tasks, with the menu open,
// N/C/D are genuinely offered and reachable — the reference must still say so, not just the four
// keys focus remaps.
func TestHelpModalStillWorksAlongsideTheMenuWhileFocused(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 0
	m.state = api.BoardState{Tasks: []api.Task{{ID: "td-1", Title: "a task", Status: "open"}}}
	m.reclamp()
	m.focus = focusItems

	m.onKey(keyMenu)
	if !m.menuAccepts(keyNew) {
		t.Fatal("precondition: N should be live in the menu on an open task")
	}
	m.onKey(keyMenu) // close it again before opening the modal

	m.onKey(keyHelp)
	got := strings.Join(m.modalLines(), "\n")
	for _, want := range []string{keyMenuShown + " " + keyNew + "  new", keyMenuShown + " " + keyClose + "  close", keyMenuShown + " " + keyDelete + "  scrap"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q should still be listed while focused, got:\n%s", want, got)
		}
	}
}

// TestReferenceListsRefOnlyBindings is refOnly's whole point: esc and enter are real dispatcher
// keys the footer never carried, and the reference must still list them — global for esc (its
// meaning does not vary by tab), per-tab for enter (Tasks' "full screen" is not what it does on
// Chat).
func TestReferenceListsRefOnlyBindings(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 0 // Tasks
	m.onKey(keyHelp)
	got := strings.Join(m.modalLines(), "\n")
	if !strings.Contains(got, keyClearFilters+"  clear filters") {
		t.Errorf("the reference should list esc, global, got:\n%s", got)
	}
	if !strings.Contains(got, keyEnter+"  full screen") {
		t.Errorf("the reference should list Tasks' own enter/full screen, got:\n%s", got)
	}
}

// TestRefOnlyBindingsStayOutOfTheFooter is the other half: refOnly exists so the footer keeps
// omitting a real dispatcher key it never advertised. Deleting the refOnly check from footerFor or
// globalFooter must fail this, not just slip through green.
func TestRefOnlyBindingsStayOutOfTheFooter(t *testing.T) {
	for _, tab := range []int{0, 1, 2, 5, 6} { // every scope carrying its own enter/expand row
		m := newModel(nil, nil, "/r/one")
		m.tab = tab
		if local := m.contextFooter(); strings.Contains(local, keyEnter) {
			t.Errorf("tab %d: the local footer should not carry enter/expand, got %q", tab, local)
		}
		if global := m.globalFooter(500); strings.Contains(global, keyClearFilters) {
			t.Errorf("tab %d: the global footer should not carry esc/clear-filters, got %q", tab, global)
		}
	}
}
