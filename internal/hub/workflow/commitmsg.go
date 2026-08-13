// package: hub/workflow / commitmsg
// type:    logic (Conventional Commits formatting)
// job:     compose the message for every commit sindri creates: infer a type from a
// task's declared type, scope it to the task id, and describe it with the agent's own
// summary when given or the task's title otherwise.
// limits:  pure formatting — no store reads, no git; callers already have the task's
// type, id and description in hand.
package workflow

import "fmt"

// ccType maps a task's declared type to the Conventional Commits type that signals it
// correctly to a changelog tool. bug and an imported issue read as a fix; feature and
// epic read as a feature; anything else — chore, task, and an unknown or missing type —
// reads as routine maintenance.
func ccType(taskType string) string {
	switch taskType {
	case "bug", "issue":
		return "fix"
	case "feature", "epic":
		return "feat"
	default:
		return "chore"
	}
}

// conventionalCommit composes type(taskID): desc, the shape a Conventional Commits
// linter accepts. taskID becomes the scope so the id stays grep-able once desc is
// prose rather than an id itself; with no taskID (the openspec branch, which is not
// one task) the scope is omitted rather than left empty.
func conventionalCommit(taskType, taskID, desc string) string {
	t := ccType(taskType)
	if taskID == "" {
		return t + ": " + desc
	}
	return fmt.Sprintf("%s(%s): %s", t, taskID, desc)
}
