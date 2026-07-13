// package: hub / comments
// type:    logic (comments wiring)
// job:     wire the extracted comments module (internal/hub/comments) into the hub —
//
//	provide its Deps seam (project-root resolution + board notify) and the
//	thin method the server calls. The sync/reconcile logic lives in the module.
//
// limits:  no comment logic here (-> internal/hub/comments); just the seam + delegation.
package hub

// commentsDeps adapts the hub to comments.Deps: resolve a project's root path and
// wake the board — the only hooks the comments module needs from the hub.
type commentsDeps struct{ h *Hub }

func (c commentsDeps) ProjectRoot(project string) string { return c.h.projectRoot(project) }
func (c commentsDeps) Notify()                           { c.h.notify() }

// RefreshTaskComments forces a re-sync of one task's comments (the [r]efresh key).
// Kept on the hub so the server route is unchanged.
func (h *Hub) RefreshTaskComments(project, id string) error {
	return h.comments.Refresh(project, id)
}
