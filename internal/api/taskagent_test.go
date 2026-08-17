package api

import "testing"

// TestASubmittedTaskStillHasAnOwner is the gap this closes. A worker submits, the PR goes for
// review, and "who submitted this" is exactly the question a reader opens the task with — the one
// the detail could not answer once nothing held the task any more.
func TestASubmittedTaskStillHasAnOwner(t *testing.T) {
	prs := []PR{{ID: "pr-1", Task: "sd-1", Agent: "bombur", Status: "open"}}
	if got := AgentOnTask(nil, prs, "sd-1"); got != "bombur" {
		t.Errorf("agent on a submitted task = %q, want its PR's author", got)
	}
}

// TestALiveClaimOutranksAnOldPR: a rejected task goes back to its worker while the PR it was
// rejected from is still on the board, so the agent holding it now is the answer.
func TestALiveClaimOutranksAnOldPR(t *testing.T) {
	agents := []AgentView{{Name: "nori", Task: "sd-1"}}
	prs := []PR{{ID: "pr-1", Task: "sd-1", Agent: "bombur", Status: "rejected"}}
	if got := AgentOnTask(agents, prs, "sd-1"); got != "nori" {
		t.Errorf("agent = %q, want the one working it now", got)
	}
}

// TestAFeatureNamesTheAgentHoldingIt: a worker on a feature carries the container in its own
// state, not the task, so a rule that read only the task column left the feature row unmarked and
// its detail unowned while somebody was plainly working it.
func TestAFeatureNamesTheAgentHoldingIt(t *testing.T) {
	agents := []AgentView{{Name: "dvalin", Task: "sd-2", Feature: "sd-1"}}
	by := AgentsByTask(agents, nil)
	if by["sd-1"] != "dvalin" {
		t.Errorf("feature owner = %q, want dvalin", by["sd-1"])
	}
	if by["sd-2"] != "dvalin" {
		t.Errorf("subtask owner = %q, want dvalin — it is the task in hand", by["sd-2"])
	}
}

// TestAFinishedPRNamesNobody: a merged or scrapped PR is history, and a closed task's row would
// otherwise keep a marker for work that ended.
func TestAFinishedPRNamesNobody(t *testing.T) {
	for _, status := range []string{"merged", "scrapped"} {
		prs := []PR{{ID: "pr-1", Task: "sd-1", Agent: "bombur", Status: status}}
		if got := AgentOnTask(nil, prs, "sd-1"); got != "" {
			t.Errorf("%s PR named %q; nobody is working that task", status, got)
		}
	}
}

// TestATaskNobodyHoldsNamesNobody, so a caller can print the field without checking twice.
func TestATaskNobodyHoldsNamesNobody(t *testing.T) {
	agents := []AgentView{{Name: "nori", Task: "sd-9"}}
	if got := AgentOnTask(agents, nil, "sd-1"); got != "" {
		t.Errorf("agent on an unheld task = %q, want none", got)
	}
}
