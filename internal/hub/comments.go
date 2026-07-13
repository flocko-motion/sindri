// package: hub / comments
// type:    logic (comments wiring)
// job:     wire the extracted comments module (internal/hub/comments) into the hub —
//
//	provide its Deps seam (project-root resolution + board notify). Everything
//	else calls h.comments directly; the sync/reconcile logic lives in the module.
//
// limits:  no comment logic here (-> internal/hub/comments); just the seam.
package hub

// commentsDeps adapts the hub to comments.Deps: resolve a project's root path and
// wake the board — the only hooks the comments module needs from the hub.
type commentsDeps struct{ h *Hub }

func (c commentsDeps) ProjectRoot(project string) string { return c.h.projectRoot(project) }
func (c commentsDeps) Notify()                           { c.h.notify() }
