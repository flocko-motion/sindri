// package: hub/flow/task / home_act
// type:    logic (which project a caller's tasks are in)
// job:     answer where a caller reads tasks from — its own project, or the one whose review it is
// holding, for a reviewer that lives fleet-wide.
// limits:  the project name. What is IN that project is the store's.
package task

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
)

// TaskHome is the project whose backlog a caller reads. A pooled reviewer's own project holds no
// tasks at all, so while it holds a review it reads the tasks of the project that review belongs to:
// it works for that project for as long as the review lasts, the way a freelancer does.
//
// Without this it read _global's backlog, found nothing, and reviewed against the diff alone — a
// silent loss, since judging work against its intent is the whole of the job and the intent lives in
// the task. Every other caller reads its own project, exactly as before.
func (a *Act) TaskHome(c registry.Caller) string {
	if c.Project != api.GlobalProject {
		return c.Project
	}
	project, pr, err := a.Store.ReviewingPR(c.Project, c.Agent)
	if err != nil || pr == "" || project == "" {
		return c.Project // holding no review: nothing names a project to borrow
	}
	return project
}
