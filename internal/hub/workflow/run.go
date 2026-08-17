// package: hub/workflow / run
// type:    logic (the run queue: schedule, list with derived position, cancel, reprioritise)
// job:     every operation on a queued or finished run, human and agent alike. The queue is ONE
// slot across the whole fleet, ranked together, never per project. Execution itself
// lives in execrun.go.
// limits:  scheduling, listing, and picking the next to run; durable crash-recovery is sd-bf837f's.
package workflow

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// runOutputCap bounds stored output the way a diff is capped (-> gitcmd.capLines) — the tail,
// not the head, since a hang or failure's evidence is at the end of the log.
const runOutputCap = 400

// capRunOutput keeps the last runOutputCap lines of s, marked when it trims anything — never
// silently, so a capped log cannot be mistaken for a short one.
func capRunOutput(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= runOutputCap {
		return strings.Join(lines, "\n") + "\n"
	}
	tail := lines[len(lines)-runOutputCap:]
	return fmt.Sprintf("… truncated: showing the last %d of %d lines.\n%s\n", runOutputCap, len(lines), strings.Join(tail, "\n"))
}

// newRunID mints a run id distinct from a task's ("sd-"/"td-"), so nothing that pattern-matches
// task ids ever mistakes one for the other.
func newRunID() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return "run-" + hex.EncodeToString(b[:]), nil
}

// ScheduleRun queues a command for later execution — the store row only; execution (-> ExecuteRun)
// is a separate step, triggered by the fleet's run watcher once this run reaches the front.
func (e *Engine) ScheduleRun(project, agent, command, priority, timeout string) (api.Run, error) {
	return e.putQueuedRun(project, agent, "", command, "", priority, timeout)
}

// ScheduleUserRun queues a run the human asked for, against a NAMED target — an agent's worktree,
// or the repo's own checkout — never one inferred from a working directory. It carries no agent and
// no task, so nothing about it can go stale (-> staleReason), and it executes against a COPY
// (-> repo.MaterializeRun), which is what makes the user's live checkout a safe target at all.
func (e *Engine) ScheduleUserRun(project, agent, command, priority, timeout string) (api.Run, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return api.Run{}, fmt.Errorf("say what to run")
	}
	workspace := "." // the repo's own checkout, when no agent is named
	if agent = strings.TrimSpace(agent); agent != "" {
		a, ok, err := e.store.For(project).GetAgent(agent)
		if err != nil {
			return api.Run{}, err
		}
		if !ok {
			return api.Run{}, fmt.Errorf("no agent %q in this repo — `sindri agent list` names them; omit --agent to run against the repo's own checkout", agent)
		}
		workspace = a.Workspace
	}
	return e.putRun(project, api.SenderUser, "", command, "", priority, timeout, workspace, "")
}

// putQueuedRun creates an AGENT's run, ordinary or gate alike, snapshotting its workspace and task
// so a later dequeue can tell it moved on (-> staleReason).
func (e *Engine) putQueuedRun(project, agent, kind, command, message, priority, timeout string) (api.Run, error) {
	ps := e.store.For(project)
	a, _, _ := ps.GetAgent(agent)
	st, _ := ps.GetState(agent)
	return e.putRun(project, agent, kind, command, message, priority, timeout, a.Workspace, st.Task)
}

// putRun is the one place a run row is created, whoever asked for it.
func (e *Engine) putRun(project, agent, kind, command, message, priority, timeout, workspace, task string) (api.Run, error) {
	id, err := newRunID()
	if err != nil {
		return api.Run{}, err
	}
	ps := e.store.For(project)
	if err := ps.PutRun(store.Run{
		ID: id, Agent: agent, Command: command, Status: "queued", Priority: priority, Timeout: timeout,
		Kind: kind, Message: message, Workspace: workspace, Task: task,
	}); err != nil {
		return api.Run{}, err
	}
	r, _, err := ps.GetRun(id)
	e.deps.Notify()
	return r, err
}

// CmdScheduleRun queues a command instead of running it in the pod, returning AT ONCE with its
// position — submit's same act-report-idle contract. --timeout=<duration> narrows the hard cap.
func (e *Engine) CmdScheduleRun(c registry.Caller, args []string, out io.Writer) (int, error) {
	timeout := ""
	if len(args) > 0 {
		if t, ok := strings.CutPrefix(args[0], "--timeout="); ok {
			timeout, args = t, args[1:]
		}
	}
	cmd := strings.TrimSpace(strings.Join(args, " "))
	if cmd == "" {
		fmt.Fprintln(out, "usage: run [--timeout=<duration>] <command...> — queues it; see your brief for when that's worth it over running it yourself")
		return 2, nil
	}
	r, err := e.ScheduleRun(c.Project, c.Agent, cmd, "", timeout)
	if err != nil {
		return 1, err
	}
	pos := 0
	if all, err := e.store.AllRuns("queued"); err == nil {
		pos = queuePositions(all)[r.ID]
	}
	fmt.Fprintf(out, "%s queued at position %d, budget %s. You'll be told the result — carry on with other work; a position is not a failure, so don't retry.\n",
		r.ID, pos, runTimeout(r.Timeout))
	return 0, nil
}

// queuePositions ranks every queued run and returns each one's 1-based position. A gate run
// (Kind != "") always outranks an ordinary one; within each group, priority (P0 highest, unset
// last) then creation order breaks ties. Runs not queued are absent from the result.
func queuePositions(runs []api.Run) map[string]int {
	queued := make([]api.Run, 0, len(runs))
	for _, r := range runs {
		if r.Status == "queued" {
			queued = append(queued, r)
		}
	}
	sort.SliceStable(queued, func(i, j int) bool {
		if ui, uj := api.RunFromUser(queued[i]), api.RunFromUser(queued[j]); ui != uj {
			// A user's run before every agent's, gate runs included: somebody is WAITING on it,
			// while the agent behind a gate run is parked and watching nothing. What it costs them
			// is bounded by this run's own cap, and a human left behind a queue of background
			// suites is the thing this ordering exists to prevent.
			return ui
		}
		gi, gj := queued[i].Kind != "", queued[j].Kind != ""
		if gi != gj {
			return gi // a gate run before any ordinary one, regardless of priority or arrival
		}
		pi, pj := queued[i].Priority, queued[j].Priority
		if (pi == "") != (pj == "") {
			return pj == "" // non-empty before empty
		}
		if pi != pj {
			return pi < pj
		}
		return queued[i].CreatedAt < queued[j].CreatedAt
	})
	pos := make(map[string]int, len(queued))
	for i, r := range queued {
		pos[r.ID] = i + 1
	}
	return pos
}

// FleetRuns is fleet-wide, so `run list` matches the TUI regardless of the caller's cwd —
// matching FleetPRs.
func (e *Engine) FleetRuns() ([]api.Run, error) {
	runs, err := e.store.AllRuns()
	if err != nil {
		return nil, err
	}
	reg := map[string]bool{}
	for _, p := range e.deps.KnownProjects() {
		reg[p.Tag] = true
	}
	out := make([]api.Run, 0, len(runs))
	for _, r := range runs {
		if reg[r.Project] {
			out = append(out, r)
		}
	}
	pos := queuePositions(out)
	for i := range out {
		out[i].Position = pos[out[i].ID]
	}
	return out, nil
}

// NextQueuedRun returns the fleet's next run to execute — position 1 in the same ranking `run
// list` shows an agent. ok is false when nothing is queued.
func (e *Engine) NextQueuedRun() (project, id string, ok bool) {
	all, err := e.store.AllRuns("queued")
	if err != nil || len(all) == 0 {
		return "", "", false
	}
	pos := queuePositions(all)
	for _, r := range all {
		if pos[r.ID] == 1 {
			return r.Project, r.ID, true
		}
	}
	return "", "", false
}

// RunProject finds a run's owner by id, matching PRProject — the caller's own project wins any
// id clash.
func (e *Engine) RunProject(fallback, id string) string {
	if _, ok, _ := e.store.For(fallback).GetRun(id); ok {
		return fallback
	}
	if runs, err := e.store.AllRuns(); err == nil {
		for _, r := range runs {
			if r.ID == id {
				return r.Project
			}
		}
	}
	return fallback
}

// RunInfo returns a run with its stored output and, while still queued, its fleet-wide position.
func (e *Engine) RunInfo(project, id string) (api.RunDetail, error) {
	ps := e.store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return api.RunDetail{}, err
	}
	if !ok {
		return api.RunDetail{}, fmt.Errorf("no such run %q", id)
	}
	if r.Status == "queued" {
		if all, err := e.store.AllRuns("queued"); err == nil {
			r.Position = queuePositions(all)[r.ID]
		}
	}
	output, _ := ps.RunOutput(id)
	return api.RunDetail{Run: r, Output: capRunOutput(output)}, nil
}

// ReprioritiseRun moves a queued run within the queue. Refused once it is no longer queued —
// running or finished, there is nothing left to reorder.
func (e *Engine) ReprioritiseRun(project, id, priority string) error {
	ps := e.store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such run %q", id)
	}
	if r.Status != "queued" {
		return fmt.Errorf("%s is %s — only a queued run can be reprioritised", id, r.Status)
	}
	if err := ps.SetRunPriority(id, priority); err != nil {
		return err
	}
	e.deps.Notify()
	return nil
}

// CmdShow dispatches "show" by id shape: a run id shows its status and stored output,
// everything else a PR's diff — one verb to remember for either.
func (e *Engine) CmdShow(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) > 0 && strings.HasPrefix(args[0], "run-") {
		return e.CmdShowRun(c, args, out)
	}
	return e.CmdShowPR(c, args, out)
}

// CmdShowRun prints a run's status, timing, and capped stored output — the on-request half of
// "inject a summary, store the full log" (-> MsgRunFinished carries the short form at finish time).
func (e *Engine) CmdShowRun(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: show <run-id>")
		return 2, nil
	}
	project := e.RunProject(c.Project, args[0])
	d, err := e.RunInfo(project, args[0])
	if err != nil {
		return 1, err
	}
	r := d.Run
	fmt.Fprintf(out, "%s  [%s]  %s\n", r.ID, r.Status, r.Command)
	if st, err1 := time.Parse(time.RFC3339, r.StartedAt); err1 == nil {
		if fn, err2 := time.Parse(time.RFC3339, r.FinishedAt); err2 == nil {
			fmt.Fprintf(out, "duration: %s\n", fn.Sub(st).Round(time.Second))
		}
	}
	if d.Output != "" {
		fmt.Fprintf(out, "\n%s\n", strings.TrimSpace(d.Output))
	}
	return 0, nil
}
