// package: hub/flow/run / run_act
// type:    logic (the run queue: schedule, list with derived position, cancel, reprioritise)
// job:     every operation on a queued or finished run, human and agent alike, plus the act half of
// the run flow — how a run's world is gathered, where its state is stored, and what its actions do.
// The queue is ONE slot across the whole fleet, ranked together, never per project.
// limits:  scheduling, listing, and acting. The MAP is hub/flow/run's, and execution itself is
// exec_act.go's.
package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// RunOutputCap bounds stored output the way a diff is capped (-> gitcmd.capLines) — the tail,
// not the head, since a hang or failure's evidence is at the end of the log.
const RunOutputCap = 400

// CapRunOutput keeps the last RunOutputCap lines of s, marked when it trims anything — never
// silently, so a capped log cannot be mistaken for a short one.
func CapRunOutput(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= RunOutputCap {
		return strings.Join(lines, "\n") + "\n"
	}
	tail := lines[len(lines)-RunOutputCap:]
	return fmt.Sprintf("… truncated: showing the last %d of %d lines.\n%s\n", RunOutputCap, len(lines), strings.Join(tail, "\n"))
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

// errNoCommand is what both scheduling paths answer an empty command with. The two are separate
// operations — an agent's run snapshots the workspace and task it holds, a user's names its target
// and runs against a copy — so what they share is this check and nothing else.
var errNoCommand = fmt.Errorf("say what to run")

// ScheduleRun queues an AGENT's command for later execution — the store row only; execution
// (-> ExecuteRun) is a separate step, triggered by the fleet's run watcher once this run reaches
// the front.
func (a *Act) ScheduleRun(project, agent, command, priority, timeout string) (api.Run, error) {
	if command = strings.TrimSpace(command); command == "" {
		return api.Run{}, errNoCommand
	}
	return a.PutQueuedRun(project, store.Run{Agent: agent, Command: command, Priority: priority, Timeout: timeout})
}

// ScheduleUserRun queues a run against a NAMED target — an agent's worktree or the repo's own
// checkout — and executes it against a COPY (-> repo.MaterializeRun), keeping the live checkout safe.
func (a *Act) ScheduleUserRun(project, agent, command, priority, timeout string) (api.Run, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return api.Run{}, errNoCommand
	}
	workspace := "." // the repo's own checkout, when no agent is named
	if agent = strings.TrimSpace(agent); agent != "" {
		ag, ok, err := a.Store.For(project).GetAgent(agent)
		if err != nil {
			return api.Run{}, err
		}
		if !ok {
			return api.Run{}, fmt.Errorf("no agent %q in this repo — `sindri agent list` names them; omit --agent to run against the repo's own checkout", agent)
		}
		workspace = ag.Workspace
	}
	return a.PutRun(project, store.Run{
		Agent: api.SenderUser, Command: command, Priority: priority, Timeout: timeout, Workspace: workspace,
	})
}

// PutQueuedRun creates an AGENT's run, ordinary or gate alike, snapshotting its workspace and task
// so a later dequeue can tell it moved on (-> StaleReason).
func (a *Act) PutQueuedRun(project string, r store.Run) (api.Run, error) {
	ps := a.Store.For(project)
	ag, _, _ := ps.GetAgent(r.Agent)
	st, _ := ps.GetState(r.Agent)
	r.Workspace, r.Task = ag.Workspace, st.Task
	return a.PutRun(project, r)
}

// PutRun is the one place a run row is created, whoever asked for it.
func (a *Act) PutRun(project string, r store.Run) (api.Run, error) {
	id, err := newRunID()
	if err != nil {
		return api.Run{}, err
	}
	r.ID = id
	if r.Status == "" {
		// A caller that already knows the outcome says so: a gate reusing a stored verdict must not
		// leave a queued row the watcher can pick up and run for real (-> GateRun).
		r.Status = "queued"
	}
	ps := a.Store.For(project)
	if err := ps.PutRun(r); err != nil {
		return api.Run{}, err
	}
	out, _, err := ps.GetRun(id)
	a.Deps.Notify()
	return out, err
}

// CmdScheduleRun queues a command instead of running it in the pod, returning AT ONCE with its
// position — submit's same act-report-idle contract. --timeout=<duration> narrows the hard cap.
func (a *Act) CmdScheduleRun(c registry.Caller, args []string, out io.Writer) (int, error) {
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
	r, err := a.ScheduleRun(c.Project, c.Agent, cmd, "", timeout)
	if err != nil {
		return 1, err
	}
	pos := 0
	if all, err := a.Store.AllRuns("queued"); err == nil {
		pos = QueuePositions(all)[r.ID]
	}
	fmt.Fprintf(out, "%s queued at position %d, budget %s. You'll be told the result — carry on with other work; a position is not a failure, so don't retry.\n",
		r.ID, pos, RunTimeout(r.Timeout))
	return 0, nil
}

// QueuePositions ranks queued runs — gate first, then priority, then creation order — and returns
// each one's 1-based position; runs not queued are absent from the result.
func QueuePositions(runs []api.Run) map[string]int {
	queued := make([]api.Run, 0, len(runs))
	for _, r := range runs {
		if r.Status == "queued" {
			queued = append(queued, r)
		}
	}
	sort.SliceStable(queued, func(i, j int) bool {
		if ui, uj := api.RunFromUser(queued[i]), api.RunFromUser(queued[j]); ui != uj {
			// A user's run outranks every agent's, gate runs included — a human is WAITING on it,
			// while the agent behind a gate run is merely parked.
			return ui
		}
		gi, gj := gateBlocksSomeone(queued[i]), gateBlocksSomeone(queued[j])
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

// FleetRuns is fleet-wide, so `run list` matches the TUI regardless of the caller's cwd — matching
// FleetPRs. NEWEST FIRST, because this is the human-facing listing and the thing that just happened
// was at the bottom of a growing list. The store's own reads stay oldest-first (-> store.AllRuns):
// the same line mail draws, where the fleet listing a person reads is DESC and an agent's sequential
// one is not.
func (a *Act) FleetRuns() ([]api.Run, error) {
	runs, err := a.Store.AllRuns()
	if err != nil {
		return nil, err
	}
	reg := map[string]bool{}
	for _, p := range a.Deps.KnownProjects() {
		reg[p.Tag] = true
	}
	out := make([]api.Run, 0, len(runs))
	for _, r := range runs {
		if reg[r.Project] {
			out = append(out, r)
		}
	}
	pos := QueuePositions(out)
	for i := range out {
		out[i].Position = pos[out[i].ID]
	}
	// Reversed AFTER ranking, never by asking the store for another order: position comes from
	// QueuePositions, whose final tiebreak is a second-precision timestamp, so runs queued within the
	// same second fall to sort stability — and stability reads whatever order the rows arrived in.
	slices.Reverse(out)
	return out, nil
}

// NextQueuedRun is position 1 in the ranking `run list` shows, false when nothing is queued. A READ:
// what starts a run is the map's own condition over the same ranking (-> hub/flow/run.AtFront).
func (a *Act) NextQueuedRun() (project, id string, ok bool) {
	all, err := a.Store.AllRuns("queued")
	if err != nil || len(all) == 0 {
		return "", "", false
	}
	pos := QueuePositions(all)
	for _, r := range all {
		if pos[r.ID] == 1 {
			return r.Project, r.ID, true
		}
	}
	return "", "", false
}

// RunProject finds a run's owner by id, matching PRProject — the caller's own project wins any
// id clash.
func (a *Act) RunProject(fallback, id string) string {
	if _, ok, _ := a.Store.For(fallback).GetRun(id); ok {
		return fallback
	}
	if runs, err := a.Store.AllRuns(); err == nil {
		for _, r := range runs {
			if r.ID == id {
				return r.Project
			}
		}
	}
	return fallback
}

// RunInfo returns a run with its stored output and, while still queued, its fleet-wide position.
func (a *Act) RunInfo(project, id string) (api.RunDetail, error) {
	ps := a.Store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return api.RunDetail{}, err
	}
	if !ok {
		return api.RunDetail{}, fmt.Errorf("no such run %q", id)
	}
	if r.Status == "queued" {
		if all, err := a.Store.AllRuns("queued"); err == nil {
			r.Position = QueuePositions(all)[r.ID]
		}
	}
	output, _ := ps.RunOutput(id)
	return api.RunDetail{Run: r, Output: CapRunOutput(output)}, nil
}

// ReprioritiseRun moves a queued run within the queue. Refused once it is no longer queued —
// running or finished, there is nothing left to reorder.
func (a *Act) ReprioritiseRun(project, id, priority string) error {
	ps := a.Store.For(project)
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
	a.Deps.Notify()
	return nil
}

// CmdShowRun prints a run's status, timing, and capped stored output — the on-request half of
// "inject a summary, store the full log" (-> prompts.MsgRunFinished carries the short form at finish time).
func (a *Act) CmdShowRun(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: show <run-id>")
		return 2, nil
	}
	project := a.RunProject(c.Project, args[0])
	// Checked ahead of RunInfo, whose error doesn't distinguish "not found" from a real fault.
	if _, ok, err := a.Store.For(project).GetRun(args[0]); err != nil {
		return 1, err
	} else if !ok {
		fmt.Fprintf(out, "no such run %q\n", args[0])
		return 1, nil
	}
	d, err := a.RunInfo(project, args[0])
	if err != nil {
		return 1, err
	}
	r := d.Run
	fmt.Fprintf(out, "%s  [%s]  %s\n", r.ID, r.Status, r.Command)
	if took := api.RunTook(r); took != "" {
		fmt.Fprintf(out, "duration: %s\n", took)
	}
	if d.Output != "" {
		fmt.Fprintf(out, "\n%s\n", strings.TrimSpace(d.Output))
	}
	return 0, nil
}

// --- the run flow's act half: how a run's world is read, stored and moved (-> hub/flow/run) ---

// runSubject splits "project/run-id", the identity the machine watches a run by.
func runSubject(s string) (project, id string, err error) {
	project, id, ok := strings.Cut(s, "/")
	if !ok {
		return "", "", fmt.Errorf("run subject %q is not project/run-id", s)
	}
	return project, id, nil
}

// NewRunFlow builds the machine over the run queue's map. With no beat it is ON DEMAND: it answers
// where a run stands and runs a pass when asked, and nothing happens on its own.
func (a *Act) NewRunFlow(lifetime context.Context, beat time.Duration) (machine.Machine[World], error) {
	return machine.New(lifetime, machine.Config[World]{
		States: Flow,
		Start:  Start,
		Gather: a.gatherRun,
		Stored: a.storedRunState,
		Move:   a.moveRunState,
		Do: map[string]machine.Doer[World]{
			Execute.Name: a.doExecuteRun,
			Drop.Name:    a.doDropRun,
		},
		Subjects: a.OpenRuns,
		Default:  flow.DefaultEvery,
		Tick:     beat,
		Record:   runRecorder{a},
	})
}

// OpenRuns is every run still worth watching, as "project/run-id". A finished run is a record
// rather than a subject, so the machine stops carrying it.
func (a *Act) OpenRuns() []string {
	all, err := a.Store.AllRuns("queued", "running")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(all))
	for _, r := range all {
		out = append(out, r.Project+"/"+r.ID)
	}
	return out
}

// gatherRun reads what the run's conditions look at, in one pass: the row, its place in the one
// fleet queue, whether the slot is taken, and whether it went stale while it waited.
func (a *Act) gatherRun(subject string) (World, error) {
	project, id, err := runSubject(subject)
	if err != nil {
		return World{}, err
	}
	ps := a.Store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil {
		return World{}, err
	}
	if !ok {
		return World{}, fmt.Errorf("no such run %q in %s", id, project)
	}
	w := World{Run: r, Stale: a.StaleReason(ps, r)}
	// ONE read of the fleet's runs for both questions: the ranking a queued run's position comes
	// from, and whether anything is already holding the single slot.
	all, err := a.Store.AllRuns("queued", "running")
	if err != nil {
		return w, err
	}
	w.Position = QueuePositions(all)[r.ID]
	for _, other := range all {
		if other.Status == "running" && other.ID != r.ID {
			w.SlotTaken = true
		}
	}
	return w, nil
}

// storedRunState is where a run stands. Its STATUS column already IS its state — both front-ends
// render that word — so this maps it onto the map's names rather than storing the fact twice.
func (a *Act) storedRunState(subject string) (string, time.Time, error) {
	project, id, err := runSubject(subject)
	if err != nil {
		return "", time.Time{}, err
	}
	ps := a.Store.For(project)
	r, ok, err := ps.GetRun(id)
	if err != nil || !ok {
		return "", time.Time{}, err
	}
	// What the machine stored wins over what the status claims: the two agree everywhere except the
	// states the status has no word for.
	if state, stamp, serr := ps.RunState(id); serr == nil && state != "" && api.RunOpen(r) {
		since, _ := time.Parse(time.RFC3339, stamp)
		return state, since, nil
	}
	switch r.Status {
	case "queued":
		since, _ := time.Parse(time.RFC3339, r.CreatedAt)
		return Queued, since, nil
	case "running":
		// StartedAt, not UpdatedAt: this is the moment the slot was taken, which is what tells a run
		// this hub started from one a dead hub left behind (-> machine.Orphaned).
		since, _ := time.Parse(time.RFC3339, r.StartedAt)
		return Executing, since, nil
	}
	since, _ := time.Parse(time.RFC3339, r.FinishedAt)
	return Finished, since, nil
}

// moveRunState writes a run's new state back onto the status column it came from. Finished writes
// NOTHING: the action recorded which ending it was, and one word would lose all four.
func (a *Act) moveRunState(subject, from, to, why string) error {
	project, id, err := runSubject(subject)
	if err != nil {
		return err
	}
	ps := a.Store.For(project)
	// The state is stored in its own right. Dropping has no STATUS of its own — it is the hub
	// deciding, not the run changing — and a state that cannot be stored is one the machine can
	// never enter: a stale run was decided about and then left exactly where it was.
	if err := ps.SetRunState(id, to); err != nil {
		return err
	}
	if to == Executing {
		if serr := ps.SetRunStatus(id, "running"); serr != nil {
			return serr
		}
		a.Deps.Notify()
	}
	return nil
}

// runRecorder keeps a run's passes on the record of the agent WAITING for it — where somebody
// chasing "why did my run never come back" already looks.
//
// TODO(parent): a run has no record of its own; the store logs by agent, PR or mail.
type runRecorder struct{ x *Act }

func (r runRecorder) Record(en machine.Entry) {
	project, id, err := runSubject(en.Subject)
	if err != nil {
		return
	}
	run, ok, gerr := r.x.Store.For(project).GetRun(id)
	if gerr != nil || !ok {
		return
	}
	detail := id + " " + en.State + ": " + en.Detail
	if en.Err != nil {
		detail += " — " + en.Err.Error()
	}
	_ = r.x.Store.For(project).LogPass(run.Agent, string(en.Step), en.Pass, detail)
}
