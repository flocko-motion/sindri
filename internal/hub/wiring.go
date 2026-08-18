// package: hub / wiring
// type:    logic (module wiring)
// job:     wire the hub's extracted modules into it — the seam adapters each module
// needs back to the hub (chat Delivery, comments Deps, workflow Deps). Each
// module's logic lives in its own package; this is only the glue.
// limits:  adapters only — no module logic here. The DTOs these modules exchange
// live in internal/api, which the hub and every front-end import directly.
package hub

import (
	"context"
	"io"
	"net/http"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/server"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// agentDeps adapts the hub to agent.Deps.
type agentDeps struct{ h *Hub }

func (d agentDeps) Notify()                                   { d.h.notify() }
func (d agentDeps) ContainerName(project, name string) string { return d.h.container(project, name) }
func (d agentDeps) ProjectRoot(project string) string         { return d.h.projectRoot(project) }
func (d agentDeps) ArchitectureDoc(project string) string     { return d.h.architectureDoc(project) }
func (d agentDeps) RefreshTask(project, id string) error      { return d.h.wf.RefreshTask(project, id) }
func (d agentDeps) Rehydrate(project, name string)            { d.h.rehydrate(project, name) }

func (d agentDeps) ProjectConfig(project string) (config.Config, error) {
	return d.h.projectConfig(project)
}

// chatDelivery adapts the hub to chat.Delivery.
type chatDelivery struct{ h *Hub }

func (c chatDelivery) Inject(project, name, text string) error {
	return c.h.agents.Inject(project, name, text)
}
func (c chatDelivery) InjectWhenReady(project, name, text string) error {
	return c.h.agents.InjectWhenReady(project, name, text)
}
func (c chatDelivery) Running(project, name string) bool {
	return container.Running(c.h.container(project, name))
}
func (c chatDelivery) Notify() { c.h.notify() }

// commentsDeps adapts the hub to comments.Deps.
type commentsDeps struct{ h *Hub }

func (c commentsDeps) ProjectRoot(project string) string { return c.h.projectRoot(project) }
func (c commentsDeps) Notify()                           { c.h.notify() }

// projectDeps adapts the hub to project.Deps.
type projectDeps struct{ h *Hub }

func (d projectDeps) DeleteAgent(project, name string) error {
	return d.h.agents.DeleteAgent(project, name)
}
func (d projectDeps) EnsureGitignore(root string)    { ensureGitignore(root) }
func (d projectDeps) RepoName(project string) string { return d.h.repoName(project) }
func (d projectDeps) RepoTag(root string) string     { return repoTag(root) }
func (d projectDeps) Notify()                        { d.h.notify() }

// agentchanDeps adapts the hub to agentchan.Deps: the channel owns transport, the hub behaviour.
type agentchanDeps struct{ h *Hub }

func (d agentchanDeps) Commands(project, name string) (any, error) {
	return d.h.AgentCommands(project, name)
}
func (d agentchanDeps) Directive(ctx context.Context, project, name string) (string, error) {
	return d.h.wf.AgentDirective(ctx, project, name)
}
func (d agentchanDeps) Exec(project, name string, args []string, out io.Writer) (int, error) {
	return d.h.AgentExec(project, name, args, out)
}
func (d agentchanDeps) TokenAgent(token string) (project, name string, ok bool, err error) {
	return d.h.agents.ForToken(token)
}
func (d agentchanDeps) LogRequests(label string, next http.Handler) http.Handler {
	return server.LogRequests(label, next)
}

// workflowDeps adapts the hub to workflow.Deps, so workflow need not import the hub.
type workflowDeps struct{ h *Hub }

func (d workflowDeps) ProjectRoot(project string) string { return d.h.projectRoot(project) }

func (d workflowDeps) ProjectConfig(project string) (config.Config, error) {
	return d.h.projectConfig(project)
}

func (d workflowDeps) ArchitectureDoc(project string) string { return d.h.architectureDoc(project) }

func (d workflowDeps) Container(project, name string) string { return d.h.container(project, name) }

func (d workflowDeps) Notify() { d.h.notify() }

func (d workflowDeps) Deliver(project, name, text string, del workflow.Delivery) error {
	return d.h.Deliver(project, name, text, del)
}

func (d workflowDeps) Interrupt(project, name string) error {
	return d.h.agents.Interrupt(project, name)
}

func (d workflowDeps) AgentAlive(project, name string) bool {
	return d.h.agents.AgentAlive(project, name)
}

func (d workflowDeps) SessionAlive(project, name string) bool {
	return d.h.agents.SessionAlive(project, name)
}

// AgentIdle reads the watchdog's last observation rather than probing: the sweep classifies every
// pane every few seconds anyway, and an answer taken here would cost an exec per agent per tick.
func (d workflowDeps) AgentIdle(project, name string) bool {
	l, ok := d.h.watch.get(project, name)
	return ok && l.up && l.runtime == "idle"
}

func (d workflowDeps) TaskComments(project, id string) []store.Comment {
	return d.h.comments.ForView(project, id)
}

func (d workflowDeps) AddTaskComment(project, id, author, body string) error {
	return d.h.comments.Add(project, id, author, body)
}

func (d workflowDeps) Subscribe() (chan struct{}, func()) { return d.h.events.subscribe() }

// KnownProjects is best-effort: a skipped scan self-corrects next tick (unlike the board -> State).
func (d workflowDeps) KnownProjects() []store.Project {
	ps, _ := d.h.projects.Known()
	return ps
}

func (d workflowDeps) BrokkrBin() (string, error) { return agent.BrokkrBinary() }

func (d workflowDeps) ContextUsage(project, name string) (tokens, window int, model string, ok bool) {
	return d.h.agents.ContextUsage(project, name)
}

func (d workflowDeps) CompactionThreshold(window int) int {
	return d.h.agents.CompactionThreshold(window)
}

func (d workflowDeps) CurrentModel(project, name string) string {
	return d.h.agents.CurrentModel(project, name)
}

func (d workflowDeps) ModelForTier(tier string) (string, bool) {
	return d.h.agents.ModelForTier(tier)
}

func (d workflowDeps) SetModel(project, name, model string) error {
	return d.h.agents.SetModel(project, name, model)
}

func (d workflowDeps) HoldsNothing(project, name, role string) (bool, error) {
	return d.h.agents.HoldsNothing(project, name, role)
}

func (d workflowDeps) Compact(project, name string) error {
	return d.h.agents.Compact(project, name)
}

func (d workflowDeps) FireClear(project, name string) error {
	return d.h.agents.FireClear(project, name)
}
