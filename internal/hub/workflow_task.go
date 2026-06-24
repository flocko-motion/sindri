// package: hub / workflow
// type:    logic (the act → report → idle loop + PR-as-merge-intent)
// job:     the real worker/reviewer verbs and the host merge. Tasks are a cached
//
//	read model synced from td (D15); `next` claims one and branches;
//	`submit` records a merge-intent and returns (no blocking); the
//	reviewer approves/rejects; the human merges. Verdicts are routed to
//	the owning agent's session by branch (object-mediated, D-routing).
//
// limits:  git is entirely hub-side (the agent edits /workspace, the hub commits
//
//	and merges); writes to td go through the td adapter (D15).
package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/issue"
)

// Tasks refreshes from td and returns all cached tasks (for `task list`).
func (h *Hub) Tasks() ([]store.Task, error) {
	_ = h.SyncTasks() // best-effort; fall back to cache on failure
	return h.store.AllTasks()
}

// TaskInfo returns one task, refreshed from the source of truth first (D15).
func (h *Hub) TaskInfo(id string) (store.Task, error) {
	t, err := td.Get(h.root, id)
	if err != nil {
		return store.Task{}, err
	}
	st := toStoreTask(t)
	if d, a, derr := td.Detail(h.root, id); derr == nil {
		st.Description, st.Acceptance = d, a
	}
	_ = h.store.UpsertTask(st)
	return st, nil
}

// TaskSpec is the full editable shape of a task — the payload of both create
// and edit. Empty fields mean "unset" (create) or "leave unchanged" (edit).
type TaskSpec struct {
	Title       string
	Type        string
	Priority    string // a P-code (P0…P4)
	Parent      string // parent task id (a child of this task)
	Description string
	Labels      []string
}

// CreateTask creates a task via the td tool and returns its id.
func (h *Hub) CreateTask(s TaskSpec) (string, error) {
	if err := h.checkParent(s.Parent, ""); err != nil {
		return "", err
	}
	out, err := td.Create(h.root, s.Title, td.CreateOpts{
		Type: s.Type, Priority: s.Priority, Body: s.Description, Labels: s.Labels, Parent: s.Parent,
	})
	if err != nil {
		return "", err
	}
	_ = h.SyncTasks()
	h.notify()
	// td prints e.g. "CREATED td-1add0f" — return just the id.
	id := strings.TrimSpace(out)
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, "td-") {
			id = f
			break
		}
	}
	return id, nil
}

// healPlannerTasks releases any backlog task a planner is holding in its state —
// an invalid assignment (planners can't grab tasks). Self-heals stale claims
// left by older builds; runs once at hub boot.
func (h *Hub) healPlannerTasks() {
	roster, _ := h.store.Roster()
	for _, a := range roster {
		if a.Role != "planner" {
			continue
		}
		st, _ := h.store.GetState(a.Name)
		if !strings.HasPrefix(st.Task, "td-") {
			continue
		}
		_ = td.SetStatus(h.root, st.Task, "open")
		_ = h.store.SetState(store.AgentState{Agent: a.Name, Phase: "planning"})
		_ = h.store.Log(a.Name, "unassign", st.Task+" (planners don't hold tasks)")
	}
}

// UnassignTask releases a task back to the backlog (status → open) and clears it
// from whatever agent held it. Refused if that agent is currently alive and
// working on it — stop or delete the agent first; allowed for a down agent or an
// orphaned in_progress task.
func (h *Hub) UnassignTask(id string) error {
	roster, _ := h.store.Roster()
	for _, a := range roster {
		st, _ := h.store.GetState(a.Name)
		if st.Task != id {
			continue
		}
		if h.agentAlive(a.Name) {
			return fmt.Errorf("%s is alive and working on %s — stop or delete it first", a.Name, id)
		}
		_ = h.store.SetState(store.AgentState{Agent: a.Name, Phase: "idle"})
		_ = h.store.Log(a.Name, "unassign", id)
	}
	if strings.HasPrefix(id, "td-") {
		if err := td.SetStatus(h.root, id, "open"); err != nil {
			return err
		}
	}
	_ = h.refreshTask(id)
	h.notify()
	return nil
}

// ApproveTask clears the approval gate on a planner-proposed task (user-only),
// making it claimable, and tells any running planner.
func (h *Hub) ApproveTask(id string) error {
	if err := h.store.SetApproval(id, "approved", ""); err != nil {
		return err
	}
	h.notifyPlanners(fmt.Sprintf("[user] task %s was approved — it's now in the backlog for a worker.", id))
	h.notify()
	return nil
}

// RejectTask rejects a planner-proposed task with a comment (user-only); it stays
// hidden from workers, and the comment is delivered to any running planner.
func (h *Hub) RejectTask(id, comment string) error {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		comment = "rejected"
	}
	if err := h.store.SetApproval(id, "rejected", comment); err != nil {
		return err
	}
	h.notifyPlanners(fmt.Sprintf("[user] task %s was rejected: %s", id, comment))
	h.notify()
	return nil
}

// notifyPlanners injects a message into every running planner's session.
func (h *Hub) notifyPlanners(msg string) {
	roster, _ := h.store.Roster()
	for _, a := range roster {
		if a.Role == "planner" {
			name := a.Name
			go func() { _ = h.injectWhenReady(name, msg) }()
		}
	}
}

// cmdState lets a planner flip its own resting state between "planning" (active)
// and "idle" (paused) — a planner never holds a backlog task, so it owns this.
func (h *Hub) cmdState(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) != 1 || (args[0] != "planning" && args[0] != "idle") {
		return 1, fmt.Errorf("usage: state <planning|idle>")
	}
	st, _ := h.store.GetState(c.Agent)
	st.Agent, st.Phase = c.Agent, args[0]
	if err := h.store.SetState(st); err != nil {
		return 1, err
	}
	h.notify()
	fmt.Fprintf(out, "state: %s\n", args[0])
	return 0, nil
}

// cmdCreateTask lets a planner propose a task. It's created in td but flagged
// pending the user's approval, so no worker can pick it up until the user
// approves it (planner-only — the planner's defining extra power).
func (h *Hub) cmdCreateTask(_ registry.Caller, args []string, out io.Writer) (int, error) {
	title := strings.TrimSpace(strings.Join(args, " "))
	if title == "" {
		return 1, fmt.Errorf("usage: create-task <title...>")
	}
	id, err := h.CreateTask(TaskSpec{Title: title, Type: "task"})
	if err != nil {
		return 1, err
	}
	if err := h.store.SetApproval(id, "pending", ""); err != nil {
		return 1, err
	}
	h.notify()
	fmt.Fprintln(out, replyTaskProposed(id, title))
	return 0, nil
}

// cmdTasks lets a planner read the backlog: `task list` (or no arg) lists every
// task (status, approval, priority, title); `task <id>` prints that task's full
// detail including description.
func (h *Hub) cmdTasks(_ registry.Caller, args []string, out io.Writer) (int, error) {
	_ = h.SyncTasks()
	if len(args) > 0 && args[0] != "list" {
		t, err := h.TaskInfo(args[0])
		if err != nil {
			return 1, err
		}
		appr, comment := h.store.GetApproval(t.ID)
		if comment != "" {
			appr += " — " + comment
		}
		fmt.Fprintf(out, "%s  [%s]  %s  priority=%s\napproval: %s\n\n%s\n",
			t.ID, t.Status, t.Title, dash(t.Priority), dash(appr), dash(t.Description))
		return 0, nil
	}
	tasks, err := h.store.AllTasks()
	if err != nil {
		return 1, err
	}
	for _, t := range tasks {
		fmt.Fprintf(out, "%-12s %-8s %-9s %-3s %s\n", t.ID, t.Status, dash(t.Approval), dash(t.Priority), t.Title)
	}
	return 0, nil
}

// dash renders "-" for an empty string (agent-facing output helper).
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// EditTask applies a spec to an existing task. A td task is edited through the
// td tool (its source of truth); an openspec item isn't editable as a task, so
// only its locally-assigned priority is recorded in our own db.
func (h *Hub) EditTask(id string, s TaskSpec) error {
	if err := h.checkParent(s.Parent, id); err != nil {
		return err
	}
	if strings.HasPrefix(id, "td-") {
		if err := td.Update(h.root, id, td.UpdateOpts{
			Title: s.Title, Type: s.Type, Priority: s.Priority, Body: s.Description, Labels: s.Labels, Parent: s.Parent,
		}); err != nil {
			return err
		}
	} else if s.Priority != "" {
		if err := h.store.SetPriorityOverride(id, s.Priority); err != nil {
			return err
		}
	}
	err := h.SyncTasks()
	h.notify()
	return err
}

// workPollInterval re-checks for work while a directive is parked — frequent
// enough to feel responsive, since external td edits don't notify the hub.
const workPollInterval = 3 * time.Second

// AgentDirective is the single next action the hub wants this agent to take —
// the no-arg `sindri-worker` answer. The hub decides exactly what to do next; the
// agent obeys (it never has to find work for itself, and never needs a second
// command). When there is nothing to do it BLOCKS until there is — so the agent's
// whole loop is "run `sindri-worker`, do what it says, repeat". ctx (the request)
// cancels the wait when the agent's pod dies or it disconnects.
func (h *Hub) AgentDirective(ctx context.Context, name string) (string, error) {
	a, ok, err := h.store.GetAgent(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("unknown agent %q", name)
	}
	if a.Role == "reviewer" {
		// Block until a pull request needs a verdict.
		return h.waitForWork(ctx, func() (string, bool, error) {
			prs, err := h.store.PRs()
			if err != nil {
				return "", false, err
			}
			for _, pr := range prs {
				if pr.Status == "open" {
					return dirReview(pr.ID, pr.Task), true, nil
				}
			}
			return "", false, nil
		})
	}
	st, _ := h.store.GetState(name)
	if a.Role == "planner" {
		// A planner never grabs backlog tasks. It orients and waits for the user;
		// only an in-flight openspec PR (submitted) puts it in the wait-for-verdict
		// state. Everything else → the planner brief.
		if st.Phase == "submitted" {
			return dirSubmitted, nil
		}
		return dirPlanner, nil
	}
	// A worker holding a container is in the collaborative loop: never auto-claim
	// an unrelated leaf — work the current subtask, or (subtasks exhausted) pick up
	// a newly-added child, else wait for the human to open a milestone PR.
	if st.Container != "" {
		switch st.Phase {
		case "submitted":
			return dirSubmitted, nil
		case "working":
			return dirWorking(st.Task), nil
		default:
			if next, ok := h.advanceContainer(name, st.Container); ok {
				return dirWorking(next.ID), nil
			}
			return dirContainerWait(st.Container), nil
		}
	}
	switch st.Phase {
	case "working":
		return dirWorking(st.Task), nil
	case "submitted":
		return dirSubmitted, nil
	default: // idle — claim the next task, blocking until one exists
		return h.waitForWork(ctx, func() (string, bool, error) { return h.claimNext(name) })
	}
}

// waitForWork blocks until check reports work is ready (returning its directive)
// or ctx is cancelled. It re-checks on every hub change and on a short timer, so
// it also picks up tasks created directly in td (which the hub doesn't observe).
func (h *Hub) waitForWork(ctx context.Context, check func() (string, bool, error)) (string, error) {
	ch, unsub := h.events.subscribe()
	defer unsub()
	for {
		d, ready, err := check()
		if err != nil {
			return "", err
		}
		if ready {
			return d, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ch: // a hub mutation — re-check
		case <-time.After(workPollInterval): // re-sync td and re-check
		}
	}
}

// SyncTasks refreshes the whole cached task set from its sources — td tasks and
// openspec changes — so the generalized task list is multi-source. Caches all
// statuses so UIs can filter open/closed/all client-side.
func (h *Hub) SyncTasks() error {
	tasks, err := td.Tasks(h.root, issue.FilterAll)
	if err != nil {
		return err
	}
	rows := make([]store.Task, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, toStoreTask(t))
	}
	// openspec changes as `spec`-typed tasks (source: openspec).
	for _, c := range spec.Changes(h.root) {
		status := "open"
		if c.Done() {
			status = "closed"
		}
		rows = append(rows, store.Task{
			ID:     specID(c.Name),
			Title:  fmt.Sprintf("%s (%d/%d)", c.Name, c.CompletedTasks, c.TotalTasks),
			Status: status,
			Type:   "spec",
		})
	}
	// Apply locally-assigned priorities (mainly openspec items, which have none
	// from their source).
	if ov, err := h.store.PriorityOverrides(); err == nil {
		for i := range rows {
			if p, ok := ov[rows[i].ID]; ok {
				rows[i].Priority = p
			}
		}
	}
	return h.store.ReplaceTasks(rows)
}

// SetPriority assigns a task's priority (a P-code). For a td task it writes
// through the td tool (the source); for an openspec item it records a durable
// override in our own db. Either way the cache is re-synced.
func (h *Hub) SetPriority(id, priority string) error {
	if strings.HasPrefix(id, "td-") {
		if err := td.SetPriority(h.root, id, priority); err != nil {
			return err
		}
	} else {
		if err := h.store.SetPriorityOverride(id, priority); err != nil {
			return err
		}
	}
	err := h.SyncTasks()
	h.notify()
	return err
}

// checkParent validates a requested parent id: empty is fine (a root task), it
// must not be the task itself, and it must exist in the cached task set.
func (h *Hub) checkParent(parent, self string) error {
	if parent == "" {
		return nil
	}
	if parent == self {
		return fmt.Errorf("a task can't be its own parent")
	}
	tasks, err := h.store.AllTasks()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.ID == parent {
			return nil
		}
	}
	return fmt.Errorf("unknown parent %q", parent)
}

// specID derives a stable os-XXXXXX id from an openspec change name.
func specID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "os-" + hex.EncodeToString(sum[:])[:6]
}

func (h *Hub) refreshTask(id string) error {
	t, err := td.Get(h.root, id)
	if err != nil {
		return err
	}
	return h.store.UpsertTask(toStoreTask(t))
}

func toStoreTask(t issue.Task) store.Task {
	return store.Task{
		ID: t.ID, Title: t.Title, Status: t.Status, Priority: t.Priority,
		Type: t.Type, Labels: strings.Join(t.Labels, ","), ParentID: t.ParentID,
	}
}

// cmdNext claims the highest-priority open task for a worker and branches for it.
// Tasks are refreshed from the source of truth first (refresh-before-assignment,
// D15) so a stale/closed task is never handed out.
func (h *Hub) cmdNext(c registry.Caller, _ []string, out io.Writer) (int, error) {
	d, claimed, err := h.claimNext(c.Agent)
	if err != nil {
		return 1, err
	}
	if !claimed {
		fmt.Fprintln(out, dirNoTasks)
		return 0, nil
	}
	fmt.Fprintln(out, d)
	return 0, nil
}

// claimNext claims the highest-priority open LEAF task for a worker — moving it
// to in_progress, branching in the worker's worktree, and setting the worker
// "working". Returns (directive, true) on a claim, ("", false) when nothing is
// open. Container tasks (those with children) are never auto-claimed, nor are
// children reserved to a held container — see OpenLeaves. A td sync failure is
// non-fatal (it falls back to the cached task set).
func (h *Hub) claimNext(agent string) (string, bool, error) {
	_ = h.SyncTasks() // best-effort refresh from td/openspec; cached set on failure
	// A human-marked container (a deliberate "work this whole feature with me")
	// takes priority over the leaf queue.
	if d, ok, err := h.claimContainer(agent); ok || err != nil {
		return d, ok, err
	}
	return h.claimLeaf(agent)
}

// claimLeaf claims the highest-priority open leaf for a worker, branching on the
// leaf (the structured one-task-one-branch path).
func (h *Hub) claimLeaf(agent string) (string, bool, error) {
	open, err := h.store.OpenLeaves()
	if err != nil {
		return "", false, err
	}
	if len(open) == 0 {
		return "", false, nil
	}
	t := open[0]
	base, err := h.baseBranch()
	if err != nil {
		return "", false, err
	}
	a, ok, err := h.store.GetAgent(agent)
	if err != nil || !ok {
		return "", false, fmt.Errorf("agent %s missing: %v", agent, err)
	}
	wt := filepath.Join(h.root, a.Workspace)
	branch := t.ID
	if err := td.SetStatus(h.root, t.ID, "in_progress"); err != nil {
		return "", false, err
	}
	_ = h.refreshTask(t.ID)
	if err := git.CreateBranch(wt, branch, base); err != nil {
		return "", false, err
	}
	if err := h.store.SetState(store.AgentState{Agent: agent, Task: t.ID, Branch: branch, Phase: "working"}); err != nil {
		return "", false, err
	}
	_ = h.store.Log(agent, "claim", t.ID+" "+t.Title)
	h.notify()
	return dirClaimed(t.ID, t.Title, branch), true, nil
}

// collabLabel marks a parent task for collaborative assignment: the next free
// agent takes the whole container, working its children on one standing branch.
const collabLabel = "collab"

// claimContainer assigns the highest-priority marked, unheld container to the
// agent: it puts the agent on a standing branch named for the container (created
// from base if new, preserved if it exists) and starts it on the container's
// first open child. Returns (_, false, nil) when there's no such container.
func (h *Hub) claimContainer(agent string) (string, bool, error) {
	containers, err := h.store.MarkedContainers(collabLabel)
	if err != nil || len(containers) == 0 {
		return "", false, err
	}
	c := containers[0]
	children, err := h.store.OpenChildren(c.ID)
	if err != nil {
		return "", false, err
	}
	if len(children) == 0 {
		return "", false, nil // marked but nothing open to work
	}
	base, err := h.baseBranch()
	if err != nil {
		return "", false, err
	}
	a, ok, err := h.store.GetAgent(agent)
	if err != nil || !ok {
		return "", false, fmt.Errorf("agent %s missing: %v", agent, err)
	}
	wt := filepath.Join(h.root, a.Workspace)
	if err := git.EnsureBranch(wt, c.ID, base); err != nil {
		return "", false, err
	}
	child := children[0]
	if err := td.SetStatus(h.root, child.ID, "in_progress"); err != nil {
		return "", false, err
	}
	_ = h.refreshTask(child.ID)
	if err := h.store.SetState(store.AgentState{Agent: agent, Container: c.ID, Branch: c.ID, Task: child.ID, Phase: "working"}); err != nil {
		return "", false, err
	}
	_ = h.store.Log(agent, "claim-container", c.ID+" "+c.Title)
	h.notify()
	return dirContainerClaimed(c.ID, c.Title, child.ID, child.Title), true, nil
}

// cmdCheckpoint is the collaborative worker's non-blocking verb: commit the
// current subtask to the container branch, close that child, and advance to the
// next open child — staying working, never blocking for review. When no open
// children remain the agent rests (still holding the container) until the human
// opens a milestone PR or adds more subtasks.
func (h *Hub) cmdCheckpoint(c registry.Caller, args []string, out io.Writer) (int, error) {
	st, err := h.store.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Container == "" || st.Phase != "working" || st.Task == "" {
		fmt.Fprintln(out, replyNothingToCheckpoint)
		return 1, nil
	}
	a, _, _ := h.store.GetAgent(c.Agent)
	wt := filepath.Join(h.root, a.Workspace)
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		msg = "work on " + st.Task
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	if err := td.SetStatus(h.root, st.Task, "closed"); err != nil {
		return 1, err
	}
	_ = h.refreshTask(st.Task)
	_ = h.store.Log(c.Agent, "checkpoint", st.Task)
	done := st.Task
	if next, ok := h.advanceContainer(c.Agent, st.Container); ok {
		fmt.Fprintln(out, replyCheckpointed(done, next.ID, next.Title))
		return 0, nil
	}
	// No open children left — rest holding the container; the human drives the
	// milestone PR (or adds subtasks).
	_ = h.store.SetState(store.AgentState{Agent: c.Agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
	h.notify()
	fmt.Fprintln(out, replyCheckpointedLast(done, st.Container))
	return 0, nil
}

// advanceContainer moves a held container's agent onto its next open child,
// returning (child, true) when one was assigned (state set to working) or
// (zero, false) when the container has no open children left.
func (h *Hub) advanceContainer(agent, container string) (store.Task, bool) {
	children, err := h.store.OpenChildren(container)
	if err != nil || len(children) == 0 {
		return store.Task{}, false
	}
	child := children[0]
	if err := td.SetStatus(h.root, child.ID, "in_progress"); err != nil {
		return store.Task{}, false
	}
	_ = h.refreshTask(child.ID)
	_ = h.store.SetState(store.AgentState{Agent: agent, Container: container, Branch: container, Task: child.ID, Phase: "working"})
	h.notify()
	return child, true
}
