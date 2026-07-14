// package: hub / hub
// type:    logic (the single writer / gatekeeper)
// job:     the per-repo hub — owns the SQLite store, registers agent identities,
//
//	launches pods that assume those identities, and delivers inbound
//	messages by driving tmux inside a pod (provenance-stamped). Usable
//	in-process (ephemeral) or behind the socket server (persistent).
//
// limits:  reaches external tools only via internal/adapter/{pod,tmux,git};
//
//	the agent's browser client + command surface arrive in Phase 2.
package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/config"
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

// Hub is the single global coordinator across every repo. It is the only writer
// of the store and the only thing that drives pods/tmux. Per-repo work is scoped
// by an agentKey (project + name); repos are resolved to a store handle via
// store.For(repoTag).
type Hub struct {
	store  *store.Store
	events *bus // change notifications for /events

	chat     *chat.Service     // the user's chatroom relay (internal/hub/chat)
	comments *comments.Service // task-comment sync (internal/hub/comments)
	agents   *agent.Service    // agent management: identity/auth/memory/inject/runtime/lifecycle
	wf       *workflow.Engine  // the PR/task lifecycle orchestrator (internal/hub/workflow)
	projects *project.Service  // repo-registry management (internal/hub/project)
	agentCh  *agentchan.Server // the inbound agent command channel (internal/hub/agentchan)
}

// agentKey identifies an agent within a project — the key for the hub's per-agent
// maps now that one hub serves many repos. project is a repoTag.
type agentKey struct {
	project string
	name    string
}

// repoTag is a short, stable per-repo id derived from the absolute project root.
// It scopes container names so two repos that reuse an agent name (the dwarf
// pool is small) don't collide in podman's host-global namespace. The digest is
// one-way — see repoSlug for the human-readable half.
func repoTag(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:4]) // 8 hex chars — plenty to separate repos
}

// RepoTag exposes the per-repo id (AgentView.Project) to host CLIs. State returns
// agents across every project, so a command scoped to one repo (e.g. coauthor)
// must filter its rows by RepoTag(root) — matching on the repo basename would
// collide exactly where repoTag was designed to disambiguate.
func RepoTag(root string) string { return repoTag(root) }

// repoSlug is the repo's directory name, lowercased and reduced to podman-safe
// characters, so `podman ps` is eyeballable (the digest disambiguates two repos
// that share a basename).
func repoSlug(root string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(filepath.Base(root)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		s = "repo"
	}
	if len(s) > 16 {
		s = s[:16]
	}
	return s
}

// Container is the podman container name for an agent, scoped to its repo so it
// never collides with a same-named agent in another repo:
// sindri-<slug>-<digest>-<name> (slug for humans, digest for uniqueness).
func Container(root, name string) string {
	return "sindri-" + repoSlug(root) + "-" + repoTag(root) + "-" + name
}

// projectRoot resolves a project (repoTag) to its on-disk repo root via the
// registry ("" if unknown). The hub's filesystem work (git, worktrees) needs the
// path; state needs only the tag.
func (h *Hub) projectRoot(project string) string {
	root, _ := h.projectPath(project)
	return root
}

// New opens the single global hub: ensures the central state dir exists and opens
// the one project-keyed store. Repos are registered lazily on first use (repo).
func New() (*Hub, error) {
	dir := paths.StateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir %s: %w", dir, err)
	}
	st, err := store.Open(filepath.Join(dir, "hub.db"))
	if err != nil {
		return nil, err
	}
	h := &Hub{store: st, events: newBus()}
	h.chat = chat.New(h.store, chatDelivery{h})
	h.comments = comments.New(h.store, commentsDeps{h})
	// agentCh before agents: the agent lifecycle serves/closes sockets through it.
	// agentchanDeps only reaches h.agents at request time, so the order is safe.
	h.agentCh = agentchan.New(h.store, agentchanDeps{h})
	h.agents = agent.New(h.store, agentDeps{h}, h.agentCh)
	h.wf = workflow.New(h.store, workflowDeps{h})
	h.projects = project.New(h.store, projectDeps{h})
	return h, nil
}

// repo registers a repo (idempotent) and returns its project-scoped store handle.
// It's the hub's single entry to per-repo state: the transport resolves a request's
// repo root, calls this, and works through the returned handle — so the project
// (a repoTag) is derived here, once, not threaded through every method.
func (h *Hub) repo(root string) *store.ProjectStore {
	tag := repoTag(root)
	_ = h.store.RegisterProject(tag, root)
	ensureGitignore(root) // keep .worktrees/ out of the repo's git status
	// Seed the placeholder ARCHITECTURE.md only when the project hasn't configured its
	// own `architecture` path (and only when the config is valid — a bad config
	// surfaces at the operation that needs it; we never write to a path the project named).
	if cfg, err := config.Load(root); err == nil && !cfg.ArchitectureSet {
		ensureArchitectureDoc(root)
	}
	return h.store.For(tag)
}

// hubIgnores are the patterns the hub keeps out of the repo's git: the git-owned
// agent worktrees, and `.todos/` — the task DB td rewrites on every task change. A
// tracked task DB dirties the working tree constantly, which breaks the hub's PR
// merge/rebase (that needs a clean tree), so the hub ignores it: task state is
// tactical and local, not versioned. (Hub state proper lives centrally under the
// state dir, not in the repo at all.)
var hubIgnores = []string{".worktrees/", ".todos/"}

// ensureGitignore appends any missing hub-artifact patterns to the repo's
// .gitignore (creating it if absent), idempotently — so a fresh project never
// fills lazygit/`git status` with worktree churn. Best-effort and loud on failure:
// it never blocks hub startup, but a write error is reported rather than swallowed.
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
	h.agentCh.CloseAll()
	server.FlushAccessLog() // emit any open access-log run before we go quiet
	return h.store.Close()
}

// SocketPath is the global hub's control socket.
func (h *Hub) SocketPath() string { return SocketPath() }

// ServeAgent brings an agent's command channel up (its unix socket) before its pod
// launches — the pod bind-mounts that socket, and the socket IS the agent's identity.
// The coordinator op the launch path (and tests) drive; serving lives in agentchan.
func (h *Hub) ServeAgent(project, name string) error { return h.agentCh.ServeAgent(project, name) }

// NewAgent registers an agent identity (the hub's public agent-registration op,
// driven by an in-process caller like a test). The mechanics live in hub/agent.
func (h *Hub) NewAgent(project, name, role, memory string) (string, error) {
	return h.agents.NewAgent(project, name, role, memory)
}

// rehydrate nudges a (re)launched agent to start once its pod's session is up (D13):
// it injects one kickoff telling the agent to ask the hub for work. The nudge fits
// new or resuming agents alike — AgentDirective is idempotent and state-driven, so
// `sindri` always lands it back on its current job (incl. changes while it was down,
// like a merged/rejected PR); Claude's --continue restores the prior conversation.
// Best-effort; runs in the background so it doesn't block launch.
func (h *Hub) rehydrate(project, name string) {
	// Let Claude boot to input-readiness first, or its Enter is eaten by the splash.
	time.Sleep(8 * time.Second)
	_ = h.agents.InjectWhenReady(project, name, workflow.MsgKickoff)
	// A chatroom member that just relaunched has lost the membership cue from its
	// durable prompt — remind it so it knows it can still talk to the room. Best-
	// effort: a store hiccup here shouldn't derail the rehydrate.
	if member, err := h.chat.IsMember(project, name); err != nil {
		fmt.Fprintf(os.Stderr, "hub: chat membership check for %s/%s failed: %v\n", project, name, err)
	} else if member {
		_ = h.agents.InjectWhenReady(project, name, chat.MsgReminder)
	}
}
