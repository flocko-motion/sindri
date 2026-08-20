// package: workflow / globalpool
// type:    logic (the reviewer pool spanning every project)
// job:     name the _global virtual project — the reviewer pool — and resolve which project a pooled
// reviewer currently works for. What it HOLDS is store.Store.ReviewingPR/RuledPRs, which
// every package reaches directly rather than through workflow.
// limits:  the name only; registering _global as a real project is elsewhere (-> sd-7374d3).
package workflow

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
)

// GlobalProject is the virtual project a fleet-wide reviewer lives in — api.GlobalProject under the
// name every call site in this package already uses; it crosses the wire, so it is defined there.
const GlobalProject = api.GlobalProject

// taskHome is the project whose backlog a caller reads. A pooled reviewer's own project holds no
// tasks at all, so while it holds a review it reads the tasks of the project that review belongs to:
// it works for that project for as long as the review lasts, the way a freelancer does.
//
// Without this it read _global's backlog, found nothing, and reviewed against the diff alone — a
// silent loss, since judging work against its intent is the whole of the job and the intent lives in
// the task. Every other caller reads its own project, exactly as before.
func (e *Engine) taskHome(c registry.Caller) string {
	if c.Project != GlobalProject {
		return c.Project
	}
	project, pr, err := e.store.ReviewingPR(c.Project, c.Agent)
	if err != nil || pr == "" || project == "" {
		return c.Project // holding no review: nothing names a project to borrow
	}
	return project
}
