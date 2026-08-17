// package: hub / server runs
// type:    assembly (HTTP routes for the run queue)
// job:     the /runs and /run/* endpoints — list, detail, queue one as the user,
// cancel, reprioritise — registered onto the hub's mux.
// limits:  routing and decoding only; the queue, its order and its execution are
// the workflow's (-> hub/workflow/run.go).
package hub

import (
	"net/http"

	"github.com/flo-at/sindri/internal/api"
)

// runRoutes registers the run-queue endpoints. Its own file because server.go had grown past what
// one file should hold, and the queue is the most self-contained group in it.
func (h *Hub) runRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := h.wf.FleetRuns() // fleet-wide, matching the TUI board — not cwd-scoped
		writeJSON(w, runs, err)
	})
	mux.HandleFunc("GET /run", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		d, err := h.wf.RunInfo(h.wf.RunProject(h.reqProject(r), id), id)
		writeJSON(w, d, err)
	})
	// The user queueing a run, into the same single slot an agent's goes into — a human wanting a
	// suite run otherwise has to ask an agent or run it outside the queue, which is the
	// uncoordinated concurrency the queue exists to prevent.
	mux.HandleFunc("POST /run/new", func(w http.ResponseWriter, r *http.Request) {
		var req api.ScheduleRunReq
		if !decode(w, r, &req) {
			return
		}
		run, err := h.wf.ScheduleUserRun(h.reqProject(r), req.Agent, req.Command, req.Priority, req.Timeout)
		writeJSON(w, run, err)
	})
	mux.HandleFunc("POST /run/cancel", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the run id.
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"cancelled"}, h.wf.CancelRun(h.wf.RunProject(h.reqProject(r), req.Name), req.Name))
	})
	mux.HandleFunc("POST /run/priority", func(w http.ResponseWriter, r *http.Request) {
		var req RunPriorityReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"ok"}, h.wf.ReprioritiseRun(h.wf.RunProject(h.reqProject(r), req.ID), req.ID, req.Priority))
	})
}
