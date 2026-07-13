// package: hub / wiring
// type:    logic (module wiring)
// job:     wire the hub's extracted modules into it — the seam adapters each module
//          needs back to the hub (chat Delivery, comments Deps, workflow Deps) and the
//          workflow DTO aliases the hub re-exports as its API. Each module's logic
//          lives in its own package; this is only the glue.
// limits:  adapters + aliases only — no module logic here.
package hub

import (
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/project"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// chatDelivery adapts the hub to chat.Delivery: agent injection via tmux, a pod
// liveness check, and board notifications — the only hooks the chat relay needs back.
type chatDelivery struct{ h *Hub }

func (c chatDelivery) Inject(project, name, text string) error {
	return c.h.inject(project, name, text)
}
func (c chatDelivery) Running(project, name string) bool {
	return container.Running(c.h.container(project, name))
}
func (c chatDelivery) Notify() { c.h.notify() }

// commentsDeps adapts the hub to comments.Deps: resolve a project's root path and
// wake the board — the only hooks the comments module needs from the hub.
type commentsDeps struct{ h *Hub }

func (c commentsDeps) ProjectRoot(project string) string { return c.h.projectRoot(project) }
func (c commentsDeps) Notify()                           { c.h.notify() }

// projectDeps adapts the hub to project.Deps: agent teardown (Forget frees a repo's
// agents), the filesystem seeders, the repo's display name + stable tag, and notify.
type projectDeps struct{ h *Hub }

func (d projectDeps) DeleteAgent(project, name string) error { return d.h.DeleteAgent(project, name) }
func (d projectDeps) EnsureGitignore(root string)           { ensureGitignore(root) }
func (d projectDeps) EnsureArchitectureDoc(root string)     { ensureArchitectureDoc(root) }
func (d projectDeps) RepoName(project string) string        { return d.h.repoName(project) }
func (d projectDeps) RepoTag(root string) string            { return repoTag(root) }
func (d projectDeps) Notify()                               { d.h.notify() }

// These are the extracted modules' API DTOs, re-exported so the hub stays the single
// facade its clients (client/TUI/CLI) import — they get the RPC
// request/response shapes from hub, alongside the other wire types in server.go,
// without reaching into the workflow package.
type (
	TaskSpec    = workflow.TaskSpec
	PRDetail    = workflow.PRDetail
	RepoSummary = project.Summary
	RepoDetail  = project.Detail
)

// workflowDeps adapts the hub to workflow.Deps: it exposes the hub facilities the
// orchestration reaches for (project resolution, agent liveness + messaging, the task
// cache refreshers, the change bus) without the workflow package depending on the hub.
type workflowDeps struct{ h *Hub }

func (d workflowDeps) ProjectRoot(project string) string { return d.h.projectRoot(project) }

func (d workflowDeps) ProjectConfig(project string) (config.Config, error) {
	return d.h.projectConfig(project)
}

func (d workflowDeps) ArchitectureDoc(project string) string { return d.h.architectureDoc(project) }

func (d workflowDeps) Container(project, name string) string { return d.h.container(project, name) }

func (d workflowDeps) Notify() { d.h.notify() }

func (d workflowDeps) InjectWhenReady(project, name, text string) error {
	return d.h.injectWhenReady(project, name, text)
}

func (d workflowDeps) AgentAlive(project, name string) bool { return d.h.agentAlive(project, name) }

func (d workflowDeps) SessionAlive(project, name string) bool {
	return d.h.sessionAlive(project, name)
}

func (d workflowDeps) TaskComments(project, id string) []store.Comment {
	return d.h.comments.ForView(project, id)
}

func (d workflowDeps) Subscribe() (chan struct{}, func()) { return d.h.events.subscribe() }

func (d workflowDeps) KnownProjects() []store.Project { return d.h.knownProjects() }

func (d workflowDeps) BrokkrBin() (string, error) { return agent.BrokkrBinary() }
