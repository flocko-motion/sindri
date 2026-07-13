// package: hub / workflow_wire
// type:    logic (workflow wiring)
// job:     wire the extracted workflow engine (internal/hub/workflow) into the hub —
//          provide its Deps seam (store access is direct; everything else the
//          orchestration needs from the hub is delegated here). The state machine
//          itself lives in the workflow package.
// limits:  no workflow logic here (-> internal/hub/workflow); just the Deps adapter.
package hub

import (
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// TaskSpec and PRDetail are the workflow module's API DTOs, re-exported so the hub
// stays the single facade its clients (client/TUI/CLI) import — they get the RPC
// request/response shapes from hub, alongside the other wire types in server.go,
// without reaching into the workflow package.
type (
	TaskSpec = workflow.TaskSpec
	PRDetail = workflow.PRDetail
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

func (d workflowDeps) RefreshTask(project, id string) error { return d.h.refreshTask(project, id) }

func (d workflowDeps) RefreshCachedTask(project, id string) { d.h.refreshCachedTask(project, id) }

func (d workflowDeps) ReconcileTask(project, id string) error { return d.h.ReconcileTask(project, id) }

func (d workflowDeps) ReconcileTasks(project string) error { return d.h.ReconcileTasks(project) }

func (d workflowDeps) TaskComments(project, id string) []store.Comment {
	return d.h.comments.ForView(project, id)
}

func (d workflowDeps) Subscribe() (chan struct{}, func()) { return d.h.events.subscribe() }

func (d workflowDeps) KnownProjects() []store.Project { return d.h.knownProjects() }

func (d workflowDeps) BrokkrBin() (string, error) { return brokkrBinary() }
