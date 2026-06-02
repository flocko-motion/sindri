// package: action / lifecycle
// type:    assembly
// job:     keeps the td task lifecycle and the openspec change lifecycle in
//          sync — auto-archive the spec when the last linked task closes
//          (or prompt when the checklist isn't done), and abandon a spec by
//          deleting the change folder + closing the linked open tasks.
// limits:  no UI (-> tui), no spec-CLI shelling outside the adapter
//          (-> adapter/spec); the decision logic is a pure function so the
//          test suite can pin every branch.
package action

import (
	"fmt"

	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/issue"
)

// SpecAfterCloseAction is what should happen to a linked spec after a td task
// linked to it closes.
type SpecAfterCloseAction string

const (
	// SpecAfterCloseNone — nothing to do: no linked spec, other open tasks
	// still carry the same spec label, or the spec is already gone.
	SpecAfterCloseNone SpecAfterCloseAction = ""
	// SpecAfterCloseArchive — every linked task is now closed and the spec's
	// own checklist is complete, so archive without prompting.
	SpecAfterCloseArchive SpecAfterCloseAction = "archive"
	// SpecAfterClosePrompt — every linked task is now closed but the spec's
	// checklist still has unchecked items; ask the user whether to archive
	// anyway, since silently archiving an incomplete spec hides work.
	SpecAfterClosePrompt SpecAfterCloseAction = "prompt"
)

// SpecAfterCloseDecision is the pure result of "given everything we know
// about the board and the spec, what should the UI do next?"
type SpecAfterCloseDecision struct {
	Action         SpecAfterCloseAction
	SpecName       string
	ChecklistDone  int
	ChecklistTotal int
}

// decideSpecAfterClose is the pure decision used by MaybeArchiveLinkedSpec.
// Splitting it from the IO makes every branch trivially testable.
func decideSpecAfterClose(closedTaskID string, allTasks []issue.Task, activeSpec *spec.Change) SpecAfterCloseDecision {
	var closed *issue.Task
	for i := range allTasks {
		if allTasks[i].ID == closedTaskID {
			closed = &allTasks[i]
			break
		}
	}
	if closed == nil {
		return SpecAfterCloseDecision{}
	}
	name := closed.SpecName()
	if name == "" {
		return SpecAfterCloseDecision{}
	}
	if activeSpec == nil {
		// Already archived (or never existed): nothing to do.
		return SpecAfterCloseDecision{}
	}
	for _, t := range allTasks {
		if t.ID == closedTaskID {
			continue
		}
		if t.IsClosed() {
			continue
		}
		if t.SpecName() == name {
			// Other linked tasks still open — leave the spec alone.
			return SpecAfterCloseDecision{}
		}
	}
	decision := SpecAfterCloseDecision{
		SpecName:       name,
		ChecklistDone:  activeSpec.CompletedTasks,
		ChecklistTotal: activeSpec.TotalTasks,
	}
	if activeSpec.TotalTasks > 0 && activeSpec.CompletedTasks < activeSpec.TotalTasks {
		decision.Action = SpecAfterClosePrompt
	} else {
		decision.Action = SpecAfterCloseArchive
	}
	return decision
}

// MaybeArchiveLinkedSpec is the IO wrapper around decideSpecAfterClose: load
// the board + the spec, then ask the pure decision what to do. The caller
// (typically the TUI) then either calls ArchiveSpec, surfaces a prompt, or
// does nothing depending on Action.
func MaybeArchiveLinkedSpec(root, closedTaskID string) (SpecAfterCloseDecision, error) {
	tasks, err := td.Tasks(root)
	if err != nil {
		return SpecAfterCloseDecision{}, fmt.Errorf("load tasks: %w", err)
	}
	// Look up the spec by the closed task's label so we don't pay for
	// `openspec list` when there's nothing to do.
	var name string
	for i := range tasks {
		if tasks[i].ID == closedTaskID {
			name = tasks[i].SpecName()
			break
		}
	}
	var active *spec.Change
	if name != "" {
		active = spec.Lookup(root, name)
	}
	return decideSpecAfterClose(closedTaskID, tasks, active), nil
}

// ArchiveSpec runs `openspec archive <name> --yes` so the spec moves under
// openspec/changes/archive/.
func ArchiveSpec(root, name string) error { return spec.Archive(root, name) }

// AbandonSpec drops a spec proposal and closes every linked open task. The
// "all or nothing" rule from the user: a half-abandoned spec (folder gone,
// tasks still open) leaves the work list lying about a spec link that no
// longer exists. Returns the task IDs that were closed so the UI can list
// them in its success notification. Refuses if the spec isn't an active
// proposal — archived specs are final, and there's nothing to abandon.
func AbandonSpec(root, name string) (closed []string, err error) {
	if c := spec.Lookup(root, name); c == nil {
		return nil, fmt.Errorf("spec %s is not an active proposal (archived or missing)", name)
	}
	tasks, err := td.Tasks(root)
	if err != nil {
		return nil, fmt.Errorf("load tasks: %w", err)
	}
	for _, t := range tasks {
		if t.IsClosed() {
			continue
		}
		if t.SpecName() != name {
			continue
		}
		if err := td.Close(root, t.ID, "spec abandoned"); err != nil {
			return closed, fmt.Errorf("close %s: %w", t.ID, err)
		}
		closed = append(closed, t.ID)
	}
	if _, err := spec.Abandon(root, name); err != nil {
		return closed, err
	}
	return closed, nil
}
