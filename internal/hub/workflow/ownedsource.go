// package: hub/workflow / ownedsource
// type:    logic (sindri's own tasks, as a task source)
// job:     present the owned_tasks table through the same Source interface openspec and
// GitHub implement, so the sync, the merge notification and the close path treat
// sindri's own tasks exactly as they treat a mirrored one.
// limits:  the id scheme and lifecycle are the workflow's; storage is hub/store's.
package workflow

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// OwnedPrefix marks the ids sindri owns. Inherited from td, whose ids are embedded in PR ids,
// branch names and agent state.
const OwnedPrefix = "td-"

// NewOwnedID mints an id for a task sindri owns: the prefix plus six hex characters, the shape td
// used and openspec still uses. Random rather than sequential, so two repos never collide and an id
// carries no ordering anyone could read meaning into.
func NewOwnedID() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate task id: %w", err)
	}
	return OwnedPrefix + hex.EncodeToString(b[:]), nil
}

// ownedSource adapts the owned_tasks table to tasks.Source. It holds the project's store rather
// than deriving one from a root, which is what separates it from a source over an external tool.
type ownedSource struct{ ps *store.ProjectStore }

// Enabled is always true: the table is sindri's own, so there is nothing to detect.
func (ownedSource) Enabled(string) bool { return true }

// Tasks lists every owned task. root and force are the interface's, meaningless for a local table.
func (s ownedSource) Tasks(string, bool) ([]task.Task, error) {
	owned, err := s.ps.OwnedTasks()
	if err != nil {
		return nil, err
	}
	out := make([]task.Task, 0, len(owned))
	for _, t := range owned {
		// Parentage is left to the sync, which lays task_parent over every source's rows alike.
		out = append(out, task.Task{
			ID: t.ID, Title: t.Title, Status: t.Status, Type: t.Type, Priority: t.Priority,
			Labels: store.LabelList(t.Labels), Description: t.Description,
		})
	}
	return out, nil
}

// OnMerged closes a task whose PR landed. The note is dropped: the PR itself records why, and this
// source has no separate history to write it into.
func (s ownedSource) OnMerged(_, taskID, _ string) error {
	if !s.owns(taskID) {
		return nil
	}
	return s.ps.SetOwnedStatus(taskID, "closed")
}

// Finish ends a task from the task list: scrap discards it, done closes it. handled is false for an
// id another source owns, so the caller keeps asking.
func (s ownedSource) Finish(_, taskID string, scrap bool) (bool, error) {
	if !s.owns(taskID) {
		return false, nil
	}
	if scrap {
		return true, s.ps.DeleteOwnedTask(taskID)
	}
	return true, s.ps.SetOwnedStatus(taskID, "closed")
}

// owns gates every mutation on both the prefix and the row, so an id this project never had is
// left to the other sources rather than reported as handled.
func (s ownedSource) owns(id string) bool {
	return strings.HasPrefix(id, OwnedPrefix) && s.ps.OwnsTask(id)
}

// applySpec overlays the non-empty fields of an edit onto a stored task, which is what makes an
// omitted field mean "leave it" rather than "clear it".
func applySpec(t *store.OwnedTask, s TaskSpec) {
	if s.Title != "" {
		t.Title = s.Title
	}
	if s.Type != "" {
		t.Type = s.Type
	}
	if s.Priority != "" {
		t.Priority = s.Priority
	}
	if s.Description != "" {
		t.Description = s.Description
	}
	if len(s.Labels) > 0 {
		t.Labels = strings.Join(s.Labels, ",")
	}
}
