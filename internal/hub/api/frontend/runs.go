// package: hub/api/frontend / runs
// type:    assembly (HTTP routes for the run queue)
// job:     the /runs and /run/* endpoints — list, detail, queue one as the user,
// cancel, reprioritise — registered onto the front-end's mux.
// limits:  routing and decoding only; the queue, its order and its execution are
// the run flow's (-> hub/flow/run).
package frontend

import (
	"github.com/flo-at/sindri/internal/hub/api/serve"
	"net/http"

	"github.com/flo-at/sindri/internal/api"
)

// runRoutes registers the run-queue endpoints. Its own file because server.go had grown past what
// one file should hold, and the queue is the most self-contained group in it.
func runRoutes(mux *http.ServeMux, h Hub) {
	mux.HandleFunc("GET /runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := h.RunFlow().FleetRuns() // fleet-wide, matching the TUI board — not cwd-scoped
		serve.WriteJSON(w, runs, err)
	})
	mux.HandleFunc("GET /run", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		d, err := h.RunFlow().RunInfo(h.RunFlow().RunProject(h.ReqProject(r), id), id)
		serve.WriteJSON(w, d, err)
	})
	// The user queueing a run, into the same single slot an agent's goes into — a human wanting a
	// suite run otherwise has to ask an agent or run it outside the queue, which is the
	// uncoordinated concurrency the queue exists to prevent.
	mux.HandleFunc("POST /run/new", func(w http.ResponseWriter, r *http.Request) {
		var req api.ScheduleRunReq
		if !serve.Decode(w, r, &req) {
			return
		}
		run, err := h.RunFlow().ScheduleUserRun(h.ReqProject(r), req.Agent, req.Command, req.Priority, req.Timeout)
		serve.WriteJSON(w, run, err)
	})
	mux.HandleFunc("POST /run/cancel", func(w http.ResponseWriter, r *http.Request) {
		var req NameReq // Name carries the run id.
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"cancelled"}, h.RunFlow().CancelRun(serve.Detached(r), h.RunFlow().RunProject(h.ReqProject(r), req.Name), req.Name))
	})
	mux.HandleFunc("POST /run/priority", func(w http.ResponseWriter, r *http.Request) {
		var req RunPriorityReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"ok"}, h.RunFlow().ReprioritiseRun(h.RunFlow().RunProject(h.ReqProject(r), req.ID), req.ID, req.Priority))
	})
}
