package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// detailField returns the detail line beginning with label, "" when the pane has none.
func detailField(m model, label string) string {
	for _, it := range m.taskItems() {
		if strings.HasPrefix(it.text, label) {
			return strings.TrimSpace(strings.TrimPrefix(it.text, label))
		}
	}
	return ""
}

// taskDetailModel is the Tasks tab with the cursor on sd-1.
func taskDetailModel(agents []api.AgentView, prs []api.PR) model {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.scopeRepo = 0, false
	m.w, m.h = 120, 40
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
		Tasks:    []api.Task{{ID: "sd-1", Title: "a task", Status: "in_progress", Type: "task"}},
		Agents:   agents,
		PRs:      prs,
	}
	m.reclamp()
	return m
}

// TestTheDetailNamesTheAgentUnderReview is the reported gap. A task whose PR is up still has an
// owner, and the reader opening it is asking exactly who submitted it — which nothing on the
// screen said once the row marker was the only trace of a worker.
func TestTheDetailNamesTheAgentUnderReview(t *testing.T) {
	m := taskDetailModel(nil, []api.PR{{ID: "pr-1", Task: "sd-1", Agent: "bombur", Status: "open"}})
	if got := detailField(m, "agent:"); !strings.HasPrefix(got, "bombur ") || !strings.Contains(got, "submitted") {
		t.Errorf("agent field = %q, want the agent whose PR is under review", got)
	}
}

// TestTheDetailNamesTheAgentWorkingIt: the ordinary case, and the one the row marker draws from —
// they read the same rule, so a marked row always has a name behind it.
func TestTheDetailNamesTheAgentWorkingIt(t *testing.T) {
	m := taskDetailModel([]api.AgentView{{Name: "nori", Task: "sd-1"}}, nil)
	if got := detailField(m, "agent:"); !strings.HasPrefix(got, "nori ") || !strings.Contains(got, "working") {
		t.Errorf("agent field = %q, want nori named as working it", got)
	}
	rows := items(m.taskRows())
	if len(rows) != 1 || !strings.Contains(rows[0].text, marksAssigned()) {
		t.Errorf("the row of a worked task must carry the marker: %q", rowTexts(rows))
	}
}

// marksAssigned is the worked-on mark as the row draws it.
func marksAssigned() string { return taskMarks(api.TaskWorking, "") }

// marksAnyAgent reports whether a row carries ANY of the agent glyphs — the question "is somebody
// behind this", which the detail pane answers in words.
func marksAnyAgent(text string) bool {
	return strings.Contains(text, theme.MarkAssigned) || strings.Contains(text, theme.MarkHolds)
}

// TestTheRowAndTheDetailAgreeOnTheOwner: they used to be two lookups over the same board, and the
// day one learned about features or PRs the other would have gone on marking nothing. It now also
// holds them to the same RELATION, since the glyph and the words are two renderings of one answer.
func TestTheRowAndTheDetailAgreeOnTheOwner(t *testing.T) {
	cases := []struct {
		name   string
		agents []api.AgentView
		prs    []api.PR
	}{
		{"worked", []api.AgentView{{Name: "nori", Task: "sd-1"}}, nil},
		{"held as a feature", []api.AgentView{{Name: "dvalin", Task: "sd-9", Feature: "sd-1"}}, nil},
		{"under review", nil, []api.PR{{ID: "pr-1", Task: "sd-1", Agent: "bombur", Status: "open"}}},
	}
	for _, c := range cases {
		m := taskDetailModel(c.agents, c.prs)
		named := detailField(m, "agent:") != "" && detailField(m, "agent:") != "-"
		marked := marksAnyAgent(items(m.taskRows())[0].text)
		if named != marked {
			t.Errorf("%s: detail names an agent=%v but the row marks one=%v", c.name, named, marked)
		}
		// And the same relation on both sides: a container drawn with the working glyph while its
		// detail says "holds this hierarchy" is the contradiction this pair exists to prevent.
		row := items(m.taskRows())[0].text
		if holds := strings.Contains(row, theme.MarkHolds); holds != strings.Contains(detailField(m, "agent:"), "holds") {
			t.Errorf("%s: row marks a held hierarchy=%v, detail says %q", c.name, holds, detailField(m, "agent:"))
		}
	}
}

// TestAnUnworkedTaskNamesNobody, so the field reads as an absence rather than as a stale name.
func TestAnUnworkedTaskNamesNobody(t *testing.T) {
	m := taskDetailModel(nil, nil)
	if got := detailField(m, "agent:"); got != "-" {
		t.Errorf("agent field on an unworked task = %q, want the placeholder", got)
	}
}

// TestAHeldHierarchyAndItsWorkedLeafReadDifferently is the reported bug. An agent that holds a
// feature and is working one subtask inside it sits against BOTH rows, and with one glyph and one
// bare name for the pair it read as two agents in one tree — a user went looking for a dispatch bug.
func TestAHeldHierarchyAndItsWorkedLeafReadDifferently(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.scopeRepo = 0, false
	m.w, m.h = 120, 40
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
		Tasks: []api.Task{
			{ID: "sd-1", Title: "the feature", Status: "in_progress", Type: "feature"},
			{ID: "sd-2", Title: "its subtask", Status: "in_progress", Type: "task", ParentID: "sd-1"},
		},
		Agents: []api.AgentView{{Name: "dain", Task: "sd-2", Feature: "sd-1"}},
	}
	m.reclamp()

	rows := items(m.taskRows())
	if len(rows) != 2 {
		t.Fatalf("want the container and its leaf, got %d row(s)", len(rows))
	}
	container, leaf := rows[0].text, rows[1].text
	if !strings.Contains(container, theme.MarkHolds) || strings.Contains(container, theme.MarkAssigned) {
		t.Errorf("the container must read as HELD, got %q", container)
	}
	if !strings.Contains(leaf, theme.MarkAssigned) || strings.Contains(leaf, theme.MarkHolds) {
		t.Errorf("the leaf must read as WORKED, got %q", leaf)
	}
}

// TestTheDetailTellsTheTwoRelationshipsApart: the same pair in words, since the marker column has
// two cells to say it in and the pane is where a reader goes to find out what a glyph meant.
func TestTheDetailTellsTheTwoRelationshipsApart(t *testing.T) {
	agents := []api.AgentView{{Name: "dain", Task: "sd-9", Feature: "sd-1"}}
	held := detailField(taskDetailModel(agents, nil), "agent:")
	if !strings.Contains(held, "dain") || !strings.Contains(held, "holds this hierarchy") {
		t.Errorf("a held container reads %q, want dain named as holding it", held)
	}
	worked := detailField(taskDetailModel([]api.AgentView{{Name: "dain", Task: "sd-1"}}, nil), "agent:")
	if held == worked {
		t.Errorf("holding and working a task both read %q — the two rows say the same thing again", held)
	}
}
