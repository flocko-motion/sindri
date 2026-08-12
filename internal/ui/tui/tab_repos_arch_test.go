package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// repoDetail renders the Repos-tab detail for a repo in the given doc state.
func repoDetail(t *testing.T, st api.RepoDocState) string {
	t.Helper()
	m := newModel(nil, nil, "/r/sindri")
	m.tab = 3
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "sin", Path: "/r/sindri"}},
		RepoDocs: map[string]api.RepoDocState{"sin": st},
	}
	m.cursor[3] = 0
	return strings.Join(m.repoDetailLines(), "\n")
}

// TestRepoDetailShowsArchitectureGap: the Repos tab is the UI for .sindri/config.yaml, so
// a repo that never told sindri where its architecture lives must see the gap here. The
// hub no longer seeds a placeholder to make the point, and a silent detail pane would hide
// a real quality loss — agents briefed with no architecture at all.
func TestRepoDetailShowsArchitectureGap(t *testing.T) {
	got := repoDetail(t, api.RepoDocState{Doc: "ARCHITECTURE.md", Advice: "no architecture doc — …"})
	if !strings.Contains(got, "no architecture doc") {
		t.Errorf("detail should surface the missing doc, got:\n%s", got)
	}
	// It must say what to do about it, and point at the key that fixes it.
	if !strings.Contains(got, "architecture") || !strings.Contains(got, "E") {
		t.Errorf("detail should point at the config key and the E hotkey, got:\n%s", got)
	}
}

// TestRepoDetailQuietWhenDocPresent: a repo in good shape gets one confirming line and no
// nagging — the recommendation exists to close a gap, not to editorialise.
func TestRepoDetailQuietWhenDocPresent(t *testing.T) {
	configured := repoDetail(t, api.RepoDocState{Doc: "docs/ARCH.md", Set: true, Readable: true})
	if !strings.Contains(configured, "docs/ARCH.md") {
		t.Errorf("detail should name the configured doc, got:\n%s", configured)
	}
	if strings.Contains(configured, "no architecture doc") || strings.Contains(configured, "⚠") {
		t.Errorf("a configured, readable doc needs no warning, got:\n%s", configured)
	}

	// Found at the default path: same, marked as the default so the user knows it wasn't
	// configured.
	def := repoDetail(t, api.RepoDocState{Doc: "ARCHITECTURE.md", Readable: true})
	if !strings.Contains(def, "ARCHITECTURE.md") || !strings.Contains(def, "default") {
		t.Errorf("a defaulted doc should be shown and marked default, got:\n%s", def)
	}
	if strings.Contains(def, "⚠") {
		t.Errorf("a readable default needs no warning, got:\n%s", def)
	}
}

// TestRepoDetailSilentWithoutSnapshot: an older hub sends no RepoDocs, so the zero value
// must read as "nothing known" rather than "no doc" — inventing a warning from an absent
// field would nag every repo.
func TestRepoDetailSilentWithoutSnapshot(t *testing.T) {
	m := newModel(nil, nil, "/r/sindri")
	m.tab = 3
	m.state = api.BoardState{Projects: []api.Project{{Tag: "sin", Path: "/r/sindri"}}}
	got := strings.Join(m.repoDetailLines(), "\n")
	if strings.Contains(got, "no architecture doc") || strings.Contains(got, "arch:") {
		t.Errorf("no snapshot should render no architecture line, got:\n%s", got)
	}
}
