package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// prYankModel is a PRs tab with one selected PR whose detail has landed, and an author whose
// worktree resolves — the state `y` is pressed in.
func prYankModel(t *testing.T) model {
	t.Helper()
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 2, false
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
		Agents:   []api.AgentView{{Name: "bombur", Project: "repo", Status: "working", Workspace: ".worktrees/bombur"}},
		PRs:      []api.PR{{ID: "pr-sd-1", Status: "open", Project: "repo", Agent: "bombur", Branch: "sd-1", Base: "main"}},
	}
	m.prDetail = api.PRDetail{
		PR:   api.PR{ID: "pr-sd-1", Status: "open", Project: "repo", Agent: "bombur", Branch: "sd-1", Base: "main"},
		Task: api.Task{ID: "sd-1", Title: "Yank a PR block", Status: "in_progress"},
		Diff: "diff --git a/x b/x\n+one\n+two",
	}
	return m
}

// TestYankAllFromThePRsListCopiesTheBlock: `Y` gives the PR, its author, its task with the title,
// and the worktree path — the fields a message or a ticket needs, and the ones nobody retypes.
// Plain `y` stays the id, as it is on every other tab.
func TestYankAllFromThePRsListCopiesTheBlock(t *testing.T) {
	m := prYankModel(t)
	m.focus = focusList

	got := strings.Join(m.prYankBlock(), "\n")
	for _, want := range []string{"pr-sd-1", "open", "bombur", "sd-1", "Yank a PR block", "/r/one/.worktrees/bombur"} {
		if !strings.Contains(got, want) {
			t.Errorf("the yanked block is missing %q:\n%s", want, got)
		}
	}
	// The identifying block: the diff belongs to the ENTER modal, and is the one thing nobody pastes.
	if strings.Contains(got, "diff --git") {
		t.Errorf("the block yank should leave the diff behind:\n%s", got)
	}
}

// TestPlainYankOnAPRCopiesTheIdAlone: `y` means "the id" everywhere else, and a PR is no exception —
// it used to hand over the whole block, so pasting an id meant editing four lines back down to one.
func TestPlainYankOnAPRCopiesTheIdAlone(t *testing.T) {
	m := prYankModel(t)
	m.focus = focusList

	m.onKey("y")

	if !strings.Contains(m.flash, "pr-sd-1") || !strings.Contains(m.flash, "copied id") {
		t.Errorf("y should copy the PR id alone, flash said %q", m.flash)
	}
}

// TestYankInTheDetailPaneIsUnchanged: with the right column focused, `y` still copies the one
// focused value. That is how a single field — the path, a task id — is lifted out on its own, and
// the list block must not take it over.
func TestYankInTheDetailPaneIsUnchanged(t *testing.T) {
	m := prYankModel(t)
	m.focus = focusItems
	for i, it := range m.prActionable() {
		if it.kind == "path" {
			m.rightCursor = i
		}
	}
	want, ok := m.focusedItem()
	if !ok || want.kind != "path" {
		t.Fatalf("expected the path item to be focusable, got %+v", want)
	}
	m.onKey("y")
	if m.flash != "copied: "+want.value {
		t.Errorf("focused yank reported %q, want the single focused value %q", m.flash, want.value)
	}
}

// TestListYankFallsBackBeforeTheDetailLands: the detail is fetched lazily, so pressing `Y` straight
// after moving the selection would otherwise paste the PREVIOUS PR's fields under the new PR's name.
// The block is withheld until the detail matches; `y` is unaffected, since an id needs no fetch.
func TestListYankFallsBackBeforeTheDetailLands(t *testing.T) {
	m := prYankModel(t)
	m.focus = focusList
	m.prDetail = api.PRDetail{PR: api.PR{ID: "pr-sd-OTHER"}} // a stale detail, as after a cursor move

	if block := m.prYankBlock(); block != nil {
		t.Errorf("a detail for another PR must not be yanked as this one's:\n%s", strings.Join(block, "\n"))
	}
	m.onKey("y")
	if m.flash != "copied id: pr-sd-1" {
		t.Errorf("y reads the row, so it answers whatever the detail is doing, got %q", m.flash)
	}
}

// TestYankedBlockOmitsAPathThatIsGone: a PR outlives its author's tree, so the path is the one
// part of the block that can be absent. The rest must still copy — a block that vanished with the
// worktree would be worse than one missing a line.
func TestYankedBlockOmitsAPathThatIsGone(t *testing.T) {
	m := prYankModel(t)
	m.state.Agents = nil // the author has been deleted; its PR remains

	got := strings.Join(m.prYankBlock(), "\n")
	if strings.Contains(got, "path:") {
		t.Errorf("no worktree remains, so no path should be claimed:\n%s", got)
	}
	for _, want := range []string{"pr-sd-1", "bombur", "Yank a PR block"} {
		if !strings.Contains(got, want) {
			t.Errorf("the rest of the block should survive a missing path, lacks %q:\n%s", want, got)
		}
	}
}

// TestPRDetailAndYankShareTheirIdentity is the drift guard the task asks for. The path lived in the
// interactive item column and in neither line builder, precisely because two places described the
// same PR. Both now derive from prIdentity, so a field added to one cannot go missing from the
// other.
func TestPRDetailAndYankShareTheirIdentity(t *testing.T) {
	m := prYankModel(t)
	full := strings.Join(m.prDetailLines(), "\n")
	for _, line := range m.prYankBlock() {
		if !strings.Contains(full, line) {
			t.Errorf("the full detail has drifted from the yanked block — it lacks:\n%s", line)
		}
	}
	// And the full detail is still the full detail: the modal reads it, and it keeps the diff.
	if !strings.Contains(full, "diff --git") {
		t.Errorf("prDetailLines lost the diff the ENTER modal shows:\n%s", full)
	}
}
