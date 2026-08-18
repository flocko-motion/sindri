// package: tui / agents_actions
// type:    ui (Agents tab — milestone, rebuild, stats)
// job:     the Agents tab's confirms and views: capturing a collaborative container's
// branch as a milestone PR, rebuilding an agent's image, the fleet's memory-use
// report, and the confirms that delete, clear, release or remove an agent.
// limits:  form/choice/view wiring only; the operations themselves are the
// hub's (-> client).
package tui

import (
	"bytes"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// openMilestoneChoice confirms before capturing an agent's container branch as a PR. It is the
// collaborative loop's one deliberate pause — the agent is blocked until the merge lands — so it
// asks rather than firing on a keystroke.
func (m *model) openMilestoneChoice(name string) {
	cl := m.cl
	m.choice = choiceModalState{
		active:  true,
		title:   "milestone PR from " + name + "? (it blocks until the merge lands)",
		options: []string{"cancel", "open a milestone PR"},
		values:  []string{"cancel", "milestone"},
		apply: func(v string) tea.Cmd {
			if v != "milestone" || cl == nil {
				return nil
			}
			return func() tea.Msg {
				pr, err := cl.MilestonePR(name)
				if err != nil {
					return errModalMsg{err}
				}
				return milestoneMsg(pr.ID)
			}
		},
	}
}

// openRebuildChoice confirms an image rebuild: it re-pulls the base and relaunches the agent, so it
// costs minutes and interrupts the session even though it resumes afterwards.
func (m *model) openRebuildChoice(name string) {
	cl := m.cl
	m.choice = choiceModalState{
		active:  true,
		title:   "rebuild the agent image for " + name + "? (re-pulls the base, then relaunches)",
		options: []string{"cancel", "rebuild and relaunch"},
		values:  []string{"cancel", "rebuild"},
		apply: func(v string) tea.Cmd {
			if v != "rebuild" || cl == nil {
				return nil
			}
			return func() tea.Msg {
				var buf bytes.Buffer
				err := cl.RebuildImage(name, &buf)
				// The build log is the answer either way: on failure it says what broke, and on
				// success it is the record that the base was actually re-pulled.
				return rebuiltMsg{name: name, log: buf.String(), err: err}
			}
		},
	}
}

// statsCmd fetches the fleet's memory use. A view, so it opens the modal rather than asking first.
func (m *model) statsCmd() tea.Cmd {
	cl := m.cl
	if cl == nil {
		return nil
	}
	m.flash = "sampling agents…"
	return func() tea.Msg {
		rep, err := cl.Stats()
		if err != nil {
			return errModalMsg{err}
		}
		return statsMsg(rep)
	}
}

// statsLines renders the report the way `agent stats` does, through the shared meter, so the two
// front-ends never disagree about a number.
func statsLines(rep api.StatsReport) []string {
	out := []string{"engine: " + rep.Engine, ""}
	if len(rep.Agents) == 0 {
		return append(out, "no running agents to sample")
	}
	out = append(out, fmt.Sprintf("%-10.10s %-12s %s", "REPO", "AGENT", "MEMORY"))
	for _, v := range rep.Agents {
		if v.Err != "" { // the reason, not a misleading zero
			out = append(out, fmt.Sprintf("%-10.10s %-12s stats unavailable: %s", v.Repo, v.Name, v.Err))
			continue
		}
		out = append(out, fmt.Sprintf("%-10.10s %-12s %s", v.Repo, v.Name, theme.MemLine(v.MemUsageBytes, v.MemLimitBytes)))
	}
	return out
}

// openDeleteChoice opens the delete-agent confirm.
func (m *model) openDeleteChoice(id string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "delete agent " + id + "?",
		options: []string{"cancel", "delete"}, values: []string{"cancel", "delete"},
		apply: func(v string) tea.Cmd {
			if v != "delete" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.DeleteAgent(id) })
		},
	}
}

// openClearContextChoice confirms arming a context clear. Always confirmed, never offered as a
// choice of when: what it destroys is everything the session remembers, including whatever the user
// typed into that pane. WHEN it lands is the agent's answer, not a question — so the title states
// it, read off the work in hand.
func (m *model) openClearContextChoice(a api.AgentView) {
	cl, name := m.cl, a.Name
	m.choice = choiceModalState{
		active: true, title: "clear " + name + "'s context?  (" + clearLandsWhen(a) + ")",
		options: []string{"cancel", "clear"}, values: []string{"cancel", "clear"},
		apply: func(v string) tea.Cmd {
			if v != "clear" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.SetClearArmed(name, true) })
		},
	}
}

// clearLandsWhen words api.ClearWaitsFor for the confirm: the rule says what the clear waits on,
// this says it in a sentence.
func clearLandsWhen(a api.AgentView) string {
	held := api.ClearWaitsFor(a)
	switch {
	case held == "":
		return "clears now — its session starts empty"
	case a.Role == "reviewer":
		return "clears when it delivers its verdict on " + held
	default:
		return "clears when it finishes " + held
	}
}

// openResumeChoice confirms clearing an agent's escalation. Confirmed rather than done on the
// keystroke because it drops the agent's own account of why it stopped: normally the agent clears its
// own once it has the answer, and this is the release for one that never will.
func (m *model) openResumeChoice(name string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "clear " + name + "'s escalation?  (it carries on without an answer)",
		options: []string{"cancel", "resume"}, values: []string{"cancel", "resume"},
		apply: func(v string) tea.Cmd {
			if v != "resume" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.ResumeAgent(name) })
		},
	}
}

// openRemoveOrphanChoice confirms a direct container rm; there's no agent identity to delete.
func (m *model) openRemoveOrphanChoice(name string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "remove orphan container " + name + "?",
		options: []string{"cancel", "remove"}, values: []string{"cancel", "remove"},
		apply: func(v string) tea.Cmd {
			if v != "remove" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.RemoveOrphan(name) })
		},
	}
}
