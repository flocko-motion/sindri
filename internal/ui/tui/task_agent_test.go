package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
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
	if got := detailField(m, "agent:"); got != "bombur" {
		t.Errorf("agent field = %q, want the agent whose PR is under review", got)
	}
}

// TestTheDetailNamesTheAgentWorkingIt: the ordinary case, and the one the row marker draws from —
// they read the same rule, so a marked row always has a name behind it.
func TestTheDetailNamesTheAgentWorkingIt(t *testing.T) {
	m := taskDetailModel([]api.AgentView{{Name: "nori", Task: "sd-1"}}, nil)
	if got := detailField(m, "agent:"); got != "nori" {
		t.Errorf("agent field = %q, want nori", got)
	}
	if rows := m.taskRows(); len(rows) != 1 || !strings.Contains(rows[0].text, marksAssigned()) {
		t.Errorf("the row of a worked task must carry the marker: %q", rows[0].text)
	}
}

// marksAssigned is the worked-on mark as the row draws it.
func marksAssigned() string { return taskMarks(true, "") }

// TestTheRowAndTheDetailAgreeOnTheOwner: they used to be two lookups over the same board, and the
// day one learned about features or PRs the other would have gone on marking nothing.
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
		marked := strings.Contains(m.taskRows()[0].text, strings.TrimSpace(marksAssigned()))
		if named != marked {
			t.Errorf("%s: detail names an agent=%v but the row marks one=%v", c.name, named, marked)
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
