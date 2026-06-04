// package: action / action
// type:    assembly
// job:     the shared PR/task mutations every interface uses — approve (also
//          satisfies review gates), merge (gate-checked, then closes the task),
//          reject (comments the reason, marks PR + siblings, reopens the task).
//          One place so the CLI, TUI, and agent CLIs cannot diverge.
// limits:  no UI and no human-confirmation prompt (those stay in the UI/command
//          layer); PRs via ghlocal/store, tasks via adapter/td, gate rules via issue.
package action

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/issue"
)

// taskIDForPR resolves the task a PR belongs to (title first, then branch).
func taskIDForPR(pr *store.PR) string {
	if id := issue.TaskIDFromTitle(pr.Title); id != "" {
		return id
	}
	return pr.Branch
}

// Approve approves a PR and satisfies its task's review gates by adding the
// matching approved-review-* labels, so a later merge is unblocked. root is the
// td project root.
func Approve(root, prID string) (*store.PR, error) {
	pr, err := store.Approve(prID)
	if err != nil {
		return nil, err
	}
	taskID := taskIDForPR(pr)
	if taskID == "" {
		return pr, nil
	}
	t, err := td.Get(root, taskID)
	if err != nil {
		return pr, fmt.Errorf("approved %s but could not load task %s: %w", pr.ID, taskID, err)
	}
	labels := append([]string{}, t.Labels...)
	changed := false
	for _, g := range t.Gates() {
		if !g.Approved {
			labels = append(labels, "approved-"+g.Name)
			changed = true
		}
	}
	if changed {
		if err := td.SetLabels(root, taskID, labels); err != nil {
			return pr, fmt.Errorf("approved %s but failed to mark gates on %s: %w", pr.ID, taskID, err)
		}
	}
	return pr, nil
}

// Merge merges an approved PR after verifying its review gates, then closes the
// task. If any gate is unmet it returns the missing gates and does NOT merge
// (pr is nil). Human confirmation, if required, is the caller's responsibility.
func Merge(root, prID string) (merged *store.PR, missing []string, err error) {
	pr, err := store.Read(prID)
	if err != nil {
		return nil, nil, err
	}
	taskID := taskIDForPR(pr)
	if taskID != "" {
		t, err := td.Get(root, taskID)
		if err != nil {
			return nil, nil, err
		}
		if m := t.MissingReviews(); len(m) > 0 {
			return nil, m, nil
		}
	}
	merged, err = store.Merge(prID)
	if err != nil {
		return nil, nil, err
	}
	if taskID != "" {
		if err := td.Close(root, taskID, "PR merged"); err != nil {
			return merged, nil, fmt.Errorf("merged %s but failed to close task %s: %w", merged.ID, taskID, err)
		}
	}
	return merged, nil, nil
}

// ApproveTask closes a task with reason "approved" — the no-PR path of the
// approve action. Tasks that don't go through a PR (no code change, e.g. a
// chore or a discussion item) still need a way to signal "this is done";
// approving them just closes. Idempotent: closing an already-closed task is
// safe, td refuses with a clear error.
func ApproveTask(root, taskID string) error {
	return td.Close(root, taskID, "approved")
}

// Reject sends a PR's task back for rework: it requires a reason, comments it on
// the task, marks the PR (and the task's other open/approved PRs) rejected, and
// returns the task to open.
func Reject(root, prID, reason string) (*store.PR, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("a rejection reason is required")
	}
	pr, err := store.Read(prID)
	if err != nil {
		return nil, err
	}
	taskID := taskIDForPR(pr)
	if taskID == "" {
		pr.Status = "rejected"
		return pr, store.Write(pr)
	}
	return pr, RejectTask(root, taskID, reason)
}

// RejectTask comments the reason on a task, marks its open/approved PRs
// rejected, and returns the task to open. A reason is required.
func RejectTask(root, taskID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("a rejection reason is required")
	}
	if err := td.Comment(root, taskID, reason); err != nil {
		return fmt.Errorf("failed to comment the reason on %s: %w", taskID, err)
	}
	if err := rejectPRsForTask(taskID); err != nil {
		return err
	}
	if err := td.Reject(root, taskID); err != nil {
		return fmt.Errorf("commented the reason but failed to reopen task %s: %w", taskID, err)
	}
	return nil
}

// rejectPRsForTask marks every open/approved PR of a task as rejected.
func rejectPRsForTask(taskID string) error {
	prs, err := store.List()
	if err != nil {
		return err
	}
	for _, p := range prs {
		if (p.Status == "open" || p.Status == "approved") && taskIDForPR(p) == taskID {
			p.Status = "rejected"
			if err := store.Write(p); err != nil {
				return err
			}
		}
	}
	return nil
}
