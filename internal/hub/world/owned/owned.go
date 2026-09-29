// package: hub/world/owned / owned
// type:    logic (sindri's own tasks, as a task source)
// job:     present the owned_tasks table through the same Source interface openspec and
// GitHub implement, so the sync, the merge notification and the close path treat
// sindri's own tasks exactly as they treat a mirrored one.
// limits:  the id scheme and lifecycle are the workflow's; storage is hub/store's.
package owned

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/world/task"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// OwnedSource adapts the owned_tasks table to tasks.Source. It holds the project's store rather
// than deriving one from a root, which is what separates it from a source over an external tool.
type OwnedSource struct{ ps *store.ProjectStore }

// Over presents one project's owned tasks as a source.
func Over(ps *store.ProjectStore) OwnedSource { return OwnedSource{ps: ps} }

// Name identifies this source for comment-thread storage.
func (OwnedSource) Name() string { return "owned" }

// ToolMissing is always false: sindri's own store needs no external tool.
func (OwnedSource) ToolMissing(string) bool { return false }

// Enabled is always true: the table is sindri's own, so there is nothing to detect.
func (OwnedSource) Enabled(string) bool { return true }

// Tasks lists every owned task. root and force are the interface's, meaningless for a local table.
func (s OwnedSource) Tasks(string, bool) ([]task.Task, error) {
	owned, err := s.ps.OwnedTasks()
	if err != nil {
		return nil, err
	}
	out := make([]task.Task, 0, len(owned))
	for _, t := range owned {
		// Parentage is left to the sync, which lays task_parent over every source's rows alike.
		updatedAt, _ := time.Parse(time.RFC3339, t.UpdatedAt) // zero value if unset or malformed
		createdAt, _ := time.Parse(time.RFC3339, t.CreatedAt)
		out = append(out, task.Task{
			ID: t.ID, Title: t.Title, Status: t.Status, Type: t.Type, Priority: t.Priority, Tier: t.Tier,
			Labels: store.LabelList(t.Labels), Description: t.Description,
			CreatedAt: createdAt, UpdatedAt: updatedAt,
		})
	}
	return out, nil
}

// OnMerged closes a task whose PR landed. The note is dropped: the PR itself records why, and this
// source has no separate history to write it into.
func (s OwnedSource) OnMerged(_, taskID, _ string) error {
	if !s.owns(taskID) {
		return nil
	}
	return s.ps.SetOwnedStatus(taskID, "closed")
}

// Finish ends a task from the task list: scrap discards it, done closes it. handled is false for an
// id another source owns, so the caller keeps asking.
func (s OwnedSource) Finish(_, taskID string, scrap bool) (bool, error) {
	if !s.owns(taskID) {
		return false, nil
	}
	if scrap {
		return true, s.ps.DeleteOwnedTask(taskID)
	}
	return true, s.ps.SetOwnedStatus(taskID, "closed")
}

// Comments: a task sindri owns has no upstream thread — the hub's own comment store already IS the
// thread for it, which is the generic fallback every source shares, not something owned specifically
// provides. ok is always false.
func (s OwnedSource) Comments(_, _ string) ([]task.Comment, bool, error) { return nil, false, nil }

// AddComment: same reason as Comments — handled is always false.
func (s OwnedSource) AddComment(_, _, _ string) (bool, error) { return false, nil }

// owns gates every mutation on both the prefix and the row, so an id this project never had is
// left to the other sources rather than reported as handled.
func (s OwnedSource) owns(id string) bool {
	return task.IsOwned(id) && s.ps.OwnsTask(id)
}

// ApplySpec overlays the non-empty fields of an edit onto a stored task, which is what makes an
// omitted field mean "leave it" rather than "clear it".
func ApplySpec(t *store.OwnedTask, s api.TaskSpec) {
	if s.Title != "" {
		t.Title = s.Title
	}
	if s.Type != "" {
		t.Type = s.Type
	}
	if s.Priority != "" {
		t.Priority = s.Priority
	}
	if s.Tier != "" {
		t.Tier = s.Tier
	}
	if s.Description != "" {
		t.Description = s.Description
	}
	if len(s.Labels) > 0 {
		t.Labels = strings.Join(s.Labels, ",")
	}
}
