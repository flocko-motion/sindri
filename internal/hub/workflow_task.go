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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

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

// AgentDirective is the single next action the hub wants this agent to take —
// the no-arg `sindri-worker` answer. The hub decides exactly what to do next;
// the agent obeys (it never has to find work for itself).
func (h *Hub) AgentDirective(name string) (string, error) {
	a, ok, err := h.store.GetAgent(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("unknown agent %q", name)
	}
	if a.Role == "reviewer" {
		prs, err := h.store.PRs()
		if err != nil {
			return "", err
		}
		for _, pr := range prs {
			if pr.Status == "open" { // awaiting a verdict
				return fmt.Sprintf("Review %s (task %s): `sindri-worker show %s` and `sindri-worker lint %s`, then `sindri-worker approve %s` — or `sindri-worker reject %s \"<reason>\"`.",
					pr.ID, pr.Task, pr.ID, pr.ID, pr.ID, pr.ID), nil
			}
		}
		return "Nothing is awaiting review. Wait — the hub will tell you when a pull request arrives.", nil
	}
	st, _ := h.store.GetState(name)
	switch st.Phase {
	case "working":
		return fmt.Sprintf("Work on task %s. When your change is committed, run `sindri-worker submit \"<summary>\"`.", st.Task), nil
	case "submitted":
		return "Your pull request is under review. Wait — the hub will tell you the verdict.", nil
	default: // idle
		return "Claim your next task: run `sindri-worker next`.", nil
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
	if err := h.SyncTasks(); err != nil {
		fmt.Fprintf(out, "warning: task sync failed (%v) — using cached tasks\n", err)
	}
	open, err := h.store.OpenTasks()
	if err != nil {
		return 1, err
	}
	if len(open) == 0 {
		fmt.Fprintln(out, "No open tasks. Wait — the hub will tell you when there is work.")
		return 0, nil
	}
	t := open[0]
	base, err := h.baseBranch()
	if err != nil {
		return 1, err
	}
	a, ok, err := h.store.GetAgent(c.Agent)
	if err != nil || !ok {
		return 1, fmt.Errorf("agent %s missing: %v", c.Agent, err)
	}
	wt := filepath.Join(h.root, a.Workspace)
	branch := t.ID
	if err := td.SetStatus(h.root, t.ID, "in_progress"); err != nil {
		return 1, err
	}
	_ = h.refreshTask(t.ID)
	if err := git.CreateBranch(wt, branch, base); err != nil {
		return 1, err
	}
	if err := h.store.SetState(store.AgentState{Agent: c.Agent, Task: t.ID, Branch: branch, Phase: "working"}); err != nil {
		return 1, err
	}
	_ = h.store.Log(c.Agent, "claim", t.ID+" "+t.Title)
	fmt.Fprintf(out, "Claimed %s: %s\nBranch:  %s (your /workspace)\nWhen done, run 'sindri-worker submit'.\n", t.ID, t.Title, branch)
	return 0, nil
}
