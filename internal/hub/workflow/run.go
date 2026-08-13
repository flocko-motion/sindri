// package: hub/workflow / run
// type:    logic (the run queue: schedule, list with derived position, cancel, reprioritise)
// job:     every operation on a queued or finished run, human and agent alike. The queue is
// ONE slot across the whole fleet, not per project, so a run's position is ranked
// against every project's queued runs together, never just its own.
// limits:  scheduling and listing only; actually executing a run (containers, cache, the
// 15-minute cap) is sd-938f23's, and enforcing one-at-a-time is sd-bf837f's.
package workflow

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

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

// ScheduleRun queues a command for later execution — the store row only; there is nothing yet
// to execute it (-> sd-938f23).
func (e *Engine) ScheduleRun(project, agent, command, priority string) (api.Run, error) {
	id, err := newRunID()
	if err != nil {
		return api.Run{}, err
	}
	ps := e.store.For(project)
	if err := ps.PutRun(store.Run{ID: id, Agent: agent, Command: command, Status: "queued", Priority: priority}); err != nil {
		return api.Run{}, err
	}
	r, _, err := ps.GetRun(id)
	e.deps.Notify()
	return r, err
}

// CmdScheduleRun is the agent-facing verb: queue a command instead of running it in the pod.
// Returns AT ONCE with the run's position — the same act-report-idle contract as submit — since
// the result (pass, fail, or timeout) only exists once something executes it.
func (e *Engine) CmdScheduleRun(c registry.Caller, args []string, out io.Writer) (int, error) {
	cmd := strings.TrimSpace(strings.Join(args, " "))
	if cmd == "" {
		fmt.Fprintln(out, "usage: run <command...> — queues it; see your brief for when that's worth it over running it yourself")
		return 2, nil
	}
	r, err := e.ScheduleRun(c.Project, c.Agent, cmd, "")
	if err != nil {
		return 1, err
	}
	pos := 0
	if all, err := e.store.AllRuns("queued"); err == nil {
		pos = queuePositions(all)[r.ID]
	}
	fmt.Fprintf(out, "%s queued at position %d. You'll be told the result — carry on with other work; a position is not a failure, so don't retry.\n", r.ID, pos)
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

// CancelRun withdraws a queued or running run. Flips the status only: actually stopping a
// container already executing is sd-938f23's to wire in once execution exists.
func (e *Engine) CancelRun(project, id string) error {
	ps := e.store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such run %q", id)
	}
	if !api.RunOpen(r) {
		return fmt.Errorf("%s is %s — already finished, nothing to cancel", id, r.Status)
	}
	if err := ps.SetRunStatus(id, "cancelled"); err != nil {
		return err
	}
	e.deps.Notify()
	return nil
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
