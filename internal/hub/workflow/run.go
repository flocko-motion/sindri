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
// is a separate step, triggered by the fleet's run watcher once this run reaches the front. It
// snapshots the agent's current workspace and task: a dequeue that finds the agent has since
// moved on to something else (-> staleReason) drops the run rather than spend the only slot
// testing against a workspace this was never meant for.
func (e *Engine) ScheduleRun(project, agent, command, priority, timeout string) (api.Run, error) {
	id, err := newRunID()
	if err != nil {
		return api.Run{}, err
	}
	ps := e.store.For(project)
	a, _, _ := ps.GetAgent(agent)
	st, _ := ps.GetState(agent)
	if err := ps.PutRun(store.Run{
		ID: id, Agent: agent, Command: command, Status: "queued", Priority: priority, Timeout: timeout,
		Workspace: a.Workspace, Task: st.Task,
	}); err != nil {
		return api.Run{}, err
	}
	r, _, err := ps.GetRun(id)
	e.deps.Notify()
	return r, err
}

// CmdScheduleRun is the agent-facing verb: queue a command instead of running it in the pod.
// Returns AT ONCE with the run's position — the same act-report-idle contract as submit — since
// the result (pass, fail, or timeout) only exists once something executes it. An optional leading
// --timeout=<duration> narrows the hub's hard cap; anything else, or nothing, defers to it.
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

// queuePositions ranks every queued run in runs — priority first (P0 highest, unset last),
// creation order breaking ties — and returns each one's 1-based position. Runs not queued are
// absent from the result, position being meaningless once a run has started or finished.
func queuePositions(runs []api.Run) map[string]int {
	queued := make([]api.Run, 0, len(runs))
	for _, r := range runs {
		if r.Status == "queued" {
			queued = append(queued, r)
		}
	}
	sort.SliceStable(queued, func(i, j int) bool {
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

// CmdShow dispatches the "show" verb by id shape: a run id shows a run's status and stored
// output, everything else a PR's diff — one verb, so fetching a run's full log on request
// (-> sd-938f23's "let it be fetched on request") needs no separate command to remember.
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
