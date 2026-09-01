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
	"time"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/observe"
	"github.com/flo-at/sindri/internal/hub/server"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// observed is the hub's standing look at one agent, assembled from what it already holds in memory:
// the watchdog's last sweep and the transient lifecycle intent. Evidence only — every judgement over
// it belongs to the orchestrator (-> hub/observe, hub/situation.Surface).
//
// Nothing observed yet is a real state during startup: the gatherer exists before the watchdog, so a
// delivery on the way up would otherwise ask a nil one. The watchdog is built LAST of the two read
// here, so this one check covers the agent service too (-> Hub.open's order).
func (h *Hub) observed(project, name string) observe.Observation {
	if h.watch == nil {
		return observe.Observation{}
	}
	l, seen := h.watch.get(project, name)
	o := observe.Observation{
		Up: l.up, Clients: l.clients, State: l.state, Digest: l.digest,
		StillSince: l.stillSince, ToolSince: l.toolSince, StateSince: l.stateSince,
		Fill: l.tokens, Window: l.window, Model: l.model,
	}
	if seen {
		o.TakenAt = l.seen
	}
	o.Launching, o.LaunchFailed, o.Stopping = h.agents.Intent(project, name)
	return o
}

// harness adapts the hub to workflow.Harness: the agent's box, and nothing that names a task.
type harness struct{ h *Hub }

func (x harness) Observe(project, name string) observe.Observation {
	return x.h.observed(project, name)
}

// Probe takes a FRESH look where Observe reports the standing one — for a caller that needs the
// answer as of now. Under the hub's lifetime, since workflow.Harness carries no context of its own.
func (x harness) Probe(project, name string) observe.Observation {
	o := x.h.observed(project, name)
	o.Up = x.h.agents.AgentAlive(x.h.lifetime, project, name)
	o.TakenAt = time.Now()
	return o
}

func (x harness) Say(project, name, text string, d workflow.Delivery) error {
	return x.h.Deliver(project, name, text, d)
}

func (x harness) Clear(ctx context.Context, project, name string) error {
	return x.h.agents.Clear(ctx, project, name)
}

func (x harness) Compact(ctx context.Context, project, name string) error {
	return x.h.agents.Compact(ctx, project, name)
}

func (x harness) SetModel(ctx context.Context, project, name, model string) error {
	return x.h.agents.SetModel(ctx, project, name, model)
}

// Interrupt and Start run under the hub's lifetime for the same reason Probe does.
func (x harness) Interrupt(project, name string) error {
	return x.h.agents.Interrupt(x.h.lifetime, project, name)
}

func (x harness) Start(project, name string) error {
	return x.h.agents.Launch(x.h.lifetime, project, name, false, false, 0, 0, io.Discard)
}

func (x harness) Container(project, name string) string { return x.h.container(project, name) }

func (x harness) ModelMatches(want, detected string) bool {
	return x.h.agents.ModelMatches(want, detected)
}

func (x harness) CompactionThreshold(window int) int {
	return x.h.agents.CompactionThreshold(window)
}

// agentDeps adapts the hub to agent.Deps.
type agentDeps struct{ h *Hub }

func (d agentDeps) Notify()                                   { d.h.notify() }
func (d agentDeps) ContainerName(project, name string) string { return d.h.container(project, name) }
func (d agentDeps) ProjectRoot(project string) string         { return d.h.projectRoot(project) }
func (d agentDeps) ArchitectureDoc(project string) string     { return d.h.architectureDoc(project) }
func (d agentDeps) RefreshTask(project, id string) error      { return d.h.wf.RefreshTask(project, id) }
func (d agentDeps) Rehydrate(project, name string)            { d.h.rehydrate(project, name) }

func (d agentDeps) Kickoff(project, name string) string { return d.h.wf.Kickoff(project, name) }

func (d agentDeps) Observation(project, name string) observe.Observation {
	return d.h.observed(project, name)
}

func (d agentDeps) Deliver(project, name, text string, del workflow.Delivery) error {
	return d.h.Deliver(project, name, text, del)
}

// ForgetFill drops the observer's fill for one agent, so the board stops reporting a figure the
// hub has just made false. Zeroed rather than re-sampled: the transcript is rewritten by the agent,
// not by us, so the honest answer until the next sweep is that nobody has measured it.
func (d agentDeps) ForgetFill(project, name string) { d.h.watch.forgetFill(project, name) }

// AgentUp and AgentClients read the watchdog's last observation, for the hub's own idle/clear ticks
// (FireIdleStops, FireArmedClears): a probe per roster member per tick is what the watchdog exists
// to spare.
func (d agentDeps) AgentUp(project, name string) bool {
	l, ok := d.h.watch.get(project, name)
	return ok && l.up
}

func (d agentDeps) AgentClients(project, name string) int {
	l, _ := d.h.watch.get(project, name)
	return l.clients
}

func (d agentDeps) ProjectConfig(project string) (config.Config, error) {
	return d.h.projectConfig(project)
}

// chatDelivery adapts the hub to chat.Delivery.
//
// Both pushes run under the hub's lifetime rather than a caller's context: chat.Delivery carries
// none, and a broadcast is fan-out to every OTHER member — work the hub owns on their behalf, which
// the sender hanging up must not cut short (-> Hub.lifetime).
type chatDelivery struct{ h *Hub }

func (c chatDelivery) Inject(project, name, text string) error {
	return c.h.agents.Inject(c.h.lifetime, project, name, text)
}
func (c chatDelivery) InjectWhenReady(project, name, text string) error {
	return c.h.agents.InjectWhenReady(c.h.lifetime, project, name, text)
}

// Running reads the watchdog's last observation, not a probe of its own — a chat broadcast checks
// every roster member, and a fresh exec per member per broadcast is exactly the cost the watchdog
// exists to spare (-> hub/watchdog.go).
func (c chatDelivery) Running(project, name string) bool {
	l, ok := c.h.watch.get(project, name)
	return ok && l.up
}
func (c chatDelivery) Notify() { c.h.notify() }

// commentsDeps adapts the hub to comments.Deps.
type commentsDeps struct{ h *Hub }

func (c commentsDeps) ProjectRoot(project string) string { return c.h.projectRoot(project) }
func (c commentsDeps) Notify()                           { c.h.notify() }

// projectDeps adapts the hub to project.Deps.
type projectDeps struct{ h *Hub }

// DeleteAgent tears the pod down under the hub's lifetime, not the request's: forgetting a repo
// deletes every agent in it, and a half-deleted one leaves a pod nobody owns (-> Hub.lifetime).
func (d projectDeps) DeleteAgent(project, name string) error {
	return d.h.agents.DeleteAgent(d.h.lifetime, project, name)
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
func (d agentchanDeps) Exec(ctx context.Context, project, name string, args []string, out io.Writer) (int, error) {
	return d.h.AgentExec(ctx, project, name, args, out)
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

func (d workflowDeps) Notify() { d.h.notify() }

func (d workflowDeps) TaskComments(project, id string) []store.Comment {
	return d.h.comments.ForView(project, id)
}

func (d workflowDeps) AddTaskComment(project, id, author, body string) error {
	return d.h.comments.Add(project, id, author, body)
}

func (d workflowDeps) Escalate(project, name, question string) (string, error) {
	return d.h.Escalate(project, name, question)
}

// KnownProjects is best-effort: a skipped scan self-corrects next tick (unlike the board -> State).
func (d workflowDeps) KnownProjects() []store.Project {
	ps, _ := d.h.projects.Known()
	return ps
}

func (d workflowDeps) ModelForTier(tier string) (string, bool) {
	return d.h.agents.ModelForTier(tier)
}
