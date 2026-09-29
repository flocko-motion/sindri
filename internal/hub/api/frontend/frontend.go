// package: hub/api/frontend / frontend
// type:    assembly (the front-end surface's seam back to the hub)
// job:     name what the routes need of the hub — the request's project, the acting half of
// each subject's flow, the services that own a subsystem, and the reads the board is
// built from — so the surface is a package the hub is handed to rather than a set of
// methods on it.
// limits:  the seam and the route table. Every rule behind a route belongs to the subject it
// asks (-> hub/flow/*, hub/messaging/*), and the agents' own surface is served
// elsewhere (-> hub/api/agents).
package frontend

import (
	"context"
	"fmt"
	"net/http"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/serve"
	"github.com/flo-at/sindri/internal/hub/comments"
	agentflow "github.com/flo-at/sindri/internal/hub/flow/agent"
	"github.com/flo-at/sindri/internal/hub/flow/fleet"
	prflow "github.com/flo-at/sindri/internal/hub/flow/pr"
	runflow "github.com/flo-at/sindri/internal/hub/flow/run"
	taskflow "github.com/flo-at/sindri/internal/hub/flow/task"
	"github.com/flo-at/sindri/internal/hub/harness"
	"github.com/flo-at/sindri/internal/hub/messaging/chat"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/project"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// Hub is everything the front-end surface asks of the hub. An interface rather than the hub
// itself, so this package states its own needs and the two surfaces can be checked for reaching
// into each other (-> internal/arch).
type Hub interface {
	// ReqProject and AgentReq resolve which repo a request concerns: the caller's own, and the
	// one holding the agent it names — the roster is fleet-wide, so the two differ.
	ReqProject(r *http.Request) string
	AgentReq(r *http.Request, name string) string

	// The acting half of each subject's flow, which is where a route's work is actually done.
	PRFlow() *prflow.Act
	TaskFlow() *taskflow.Act
	RunFlow() *runflow.Act
	AgentFlow() *agentflow.Act
	Fleet() *fleet.Engine

	// The services owning a subsystem outright.
	Agents() *harness.Service
	Projects() *project.Service
	Chat() *chat.Service
	Mail() *mail.Box
	Comments() *comments.Service

	// The board and its logs, assembled by the hub across every module.
	State(selected string) (api.BoardState, error)
	Stats(ctx context.Context) (api.StatsReport, error)
	Log(project, name string) ([]store.Event, error)
	StateLog(project, name string) ([]store.StateEvent, error)
	Refresh(project string) error
	SetRetired(project, name string, retired bool) error
	ReopenTask(project, id, author, reason string) error

	// The two endpoints that stay open, and the snapshot the second of them sends. They keep
	// their bodies in the hub because both are written over its own change bus.
	HandleEvents(w http.ResponseWriter, r *http.Request)
	HandleChatEvents(w http.ResponseWriter, r *http.Request)
	ChatView() (api.ChatView, error)
}

// globalRoutes are the only control endpoints valid without a repo context: the board reads,
// which return global agents/PRs (and no tasks when no repo is selected). Everything else is
// repo-scoped and requires X-Sindri-Project.
var globalRoutes = map[string]bool{
	"/state": true, "/events": true, "/stats": true,
	// Registry management spans repos: listing, inspecting, and forgetting a repo operate on the
	// registry by tag, not on the caller's cwd.
	"/repos": true, "/repo": true, "/repo/forget": true, "/repo/color": true,
	// Orphan removal targets a container by its (globally-unique) name, not a repo.
	"/orphan/remove": true,
}

// RequireProject rejects a repo-scoped request that arrives without an X-Sindri-Project header
// (rather than silently acting on a phantom empty project), with a clear message. The board reads
// are exempt (see globalRoutes).
func RequireProject(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !globalRoutes[r.URL.Path] && r.Header.Get("X-Sindri-Project") == "" {
			serve.WriteJSON(w, nil, fmt.Errorf("missing repo context (X-Sindri-Project) — run this inside a repo"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
