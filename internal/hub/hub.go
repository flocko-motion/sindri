// package: hub / hub
// type:    logic (the single writer / gatekeeper)
// job:     the per-repo hub — owns the SQLite store, registers agent identities,
// launches pods that assume those identities, and delivers inbound
// messages by driving tmux inside a pod (provenance-stamped). Usable
// in-process (ephemeral) or behind the socket server (persistent).
// limits:  reaches external tools only via internal/adapter/{pod,tmux,git};
// the agent's browser client + command surface arrive in Phase 2.
package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/agentchan"
	"github.com/flo-at/sindri/internal/hub/chat"
	"github.com/flo-at/sindri/internal/hub/comments"
	"github.com/flo-at/sindri/internal/hub/project"
	"github.com/flo-at/sindri/internal/hub/server"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// Hub is the one global coordinator: sole writer of the store, sole driver of pods/tmux.
type Hub struct {
	store     *store.Store
	events    *bus      // change notifications for /events
	startedAt time.Time // process start, reported on the board so `hub status` reads uptime from it rather than the OS

	chat     *chat.Service     // the user's chatroom relay (internal/hub/chat)
	comments *comments.Service // task-comment sync (internal/hub/comments)
	agents   *agent.Service    // agent management: identity/auth/memory/inject/runtime/lifecycle
	wf       *workflow.Engine  // the PR/task lifecycle orchestrator (internal/hub/workflow)
	projects *project.Service  // repo-registry management (internal/hub/project)
	agentCh  *agentchan.Server // the inbound agent command channel (internal/hub/agentchan)
	watch    *watchdog         // agent liveness, observed on a loop (internal/hub/watchdog.go)
	refs     *refwatch         // reference-branch drift, on a slow loop (internal/hub/refwatch.go)
	creds    *credwatch        // agent credential upkeep from the host (internal/hub/credwatch.go)
	stalls   *stallwatch       // held work nobody is working on (internal/hub/stallwatch.go)
}

// agentKey identifies an agent within a project (a repoTag), one hub serving many repos.
type agentKey struct {
	project string
	name    string
}

// repoTag is a short, stable per-repo id from the absolute root. It scopes container names so two
// repos reusing an agent name don't collide in podman's host-global namespace.
// repoTag is api.RepoTag under the name every existing call site here already uses; it
// crosses the wire (AgentView.Project, every repo-scoped request), so its definition
// lives in internal/api.
func repoTag(root string) string { return api.RepoTag(root) }

// RepoTag exposes the per-repo id (AgentView.Project) to host CLIs: State spans every project, so a
// repo-scoped command must filter on this — matching the repo basename collides.
func RepoTag(root string) string { return api.RepoTag(root) }

// Container is an agent's repo-scoped podman container name; it crosses to whatever
// addresses a container directly (podman inspect/logs), so its definition lives in
// internal/container under the name every existing caller here already uses.
func Container(root, name string) string { return container.AgentContainer(root, name) }

// projectRoot resolves a project (repoTag) to its on-disk repo root via the registry ("" if unknown).
func (h *Hub) projectRoot(project string) string {
	root, _ := h.projectPath(project)
	return root
}

// New opens the single global hub and its project-keyed store; repos register lazily on first use.
func New() (*Hub, error) {
	dir := paths.StateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir %s: %w", dir, err)
	}
	st, err := store.Open(filepath.Join(dir, "hub.db"))
	if err != nil {
		return nil, err
	}
	h := &Hub{store: st, events: newBus(), startedAt: time.Now()}
	h.chat = chat.New(h.store, chatDelivery{h})
	h.comments = comments.New(h.store, commentsDeps{h})
	// agentCh before agents: the lifecycle serves sockets through it, and agentchanDeps only
	// reaches h.agents at request time.
	h.agentCh = agentchan.New(h.store, agentchanDeps{h})
	h.agents = agent.New(h.store, agentDeps{h}, h.agentCh)
	h.wf = workflow.New(h.store, workflowDeps{h})
	h.projects = project.New(h.store, projectDeps{h})
	// Last, after agents: the watchdog probes through h.agents and reads once here, so the first
	// board read has real observations.
	h.watch = newWatchdog(h)
	// After wf: it drives SyncReference, whose first pass only records where each reference stands.
	h.refs = newRefwatch(h)
	h.creds = newCredwatch(h)
	// After watch: it reads the watchdog's idle dwell, and after wf: it nudges through it.
	h.stalls = newStallwatch(h)
	return h, nil
}

// repo registers a repo (idempotent) and returns its project-scoped store handle — the hub's single
// entry to per-repo state, so the repoTag is derived here once rather than threaded through methods.
func (h *Hub) repo(root string) *store.ProjectStore {
	tag := repoTag(root)
	_ = h.store.RegisterProject(tag, root)
	ensureGitignore(root) // keep .worktrees/ out of the repo's git status
	return h.store.For(tag)
}

// hubIgnores stay out of the repo's git: git-owned agent worktrees, and `.todos/` — td rewrites that
// task DB on every change, and the dirty tree breaks the hub's PR merge/rebase.
var hubIgnores = []string{".worktrees/", ".todos/"}

// ensureGitignore appends missing hubIgnores to .gitignore; best-effort, but loud on a write error.
func ensureGitignore(root string) {
	path := filepath.Join(root, ".gitignore")
	data, _ := os.ReadFile(path) // missing file → empty, we'll create it
	existing := string(data)

	have := map[string]bool{}
	for _, line := range strings.Split(existing, "\n") {
		have[strings.Trim(strings.TrimSpace(line), "/")] = true
	}
	var missing []string
	for _, e := range hubIgnores {
		if !have[strings.Trim(e, "/")] {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString(existing)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("\n# sindri hub artifacts (agent worktrees + hub state) — not for the repo\n")
	for _, e := range missing {
		b.WriteString(e + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "hub: WARNING — could not update %s: %v\n", path, err)
	}
}

// Close shuts agent listeners and releases the store.
func (h *Hub) Close() error {
	h.watch.close()
	h.refs.close()
	h.creds.close()
	h.stalls.close()
	h.agentCh.CloseAll()
	server.FlushAccessLog() // emit any open access-log run before we go quiet
	return h.store.Close()
}

// SocketPath is the global hub's control socket.
func (h *Hub) SocketPath() string { return paths.HubSocket() }

// ServeAgent opens an agent's command socket before its pod launches — the pod bind-mounts that
// socket, and the socket IS the agent's identity. Serving itself lives in agentchan.
func (h *Hub) ServeAgent(project, name string) error { return h.agentCh.ServeAgent(project, name) }

// NewAgent registers an agent identity; the mechanics live in hub/agent.
func (h *Hub) NewAgent(project, name, role, memory string) (string, error) {
	return h.agents.NewAgent(project, name, role, memory)
}

// rehydrate injects one kickoff so a (re)launched agent asks the hub for work: AgentDirective is
// idempotent and state-driven, so new and resuming agents alike land on their current job (D13).
func (h *Hub) rehydrate(project, name string) {
	// Let Claude boot to input-readiness first, or its Enter is eaten by the splash.
	time.Sleep(8 * time.Second)
	_ = h.agents.InjectWhenReady(project, name, workflow.MsgKickoff)
	// A relaunched chatroom member lost its durable prompt's membership cue — remind it (best-effort).
	if member, err := h.chat.IsMember(project, name); err != nil {
		fmt.Fprintf(os.Stderr, "hub: chat membership check for %s/%s failed: %v\n", project, name, err)
	} else if member {
		_ = h.agents.InjectWhenReady(project, name, chat.MsgReminder)
	}
}
