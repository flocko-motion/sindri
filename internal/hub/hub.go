// package: hub / hub
// type:    logic (the single writer / gatekeeper)
// job:     the per-repo hub — owns the SQLite store, registers agent identities,
//          launches pods that assume those identities, and delivers inbound
//          messages by driving tmux inside a pod (provenance-stamped). Usable
//          in-process (ephemeral) or behind the socket server (persistent).
// limits:  reaches external tools only via internal/adapter/{pod,tmux,git};
//          the agent's browser client + command surface arrive in Phase 2.
package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// Hub is the per-repo coordinator. It is the only writer of the store and the
// only thing that drives pods/tmux.
type Hub struct {
	root  string
	store *store.Store
}

// AgentState is an agent as presented to clients: durable identity plus live
// runtime status. (Orphans and richer status arrive in later phases.)
type AgentState struct {
	store.Agent
	Running bool `json:"running"`
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// Container is the podman container name for an agent.
func Container(name string) string { return "sindri-" + name }

// session is the tmux session name for an agent (named after the agent, D4).
func session(name string) string { return name }

// New opens the hub for a project: ensures `.sindri/` exists and opens the DB.
func New(root string) (*Hub, error) {
	dir := filepath.Join(root, ".sindri")
	if err := os.MkdirAll(filepath.Join(dir, "sockets"), 0o755); err != nil {
		return nil, fmt.Errorf("create .sindri: %w", err)
	}
	st, err := store.Open(filepath.Join(dir, "hub.db"))
	if err != nil {
		return nil, err
	}
	return &Hub{root: root, store: st}, nil
}

// Close releases the store.
func (h *Hub) Close() error { return h.store.Close() }

// SocketPath is the hub's control socket for this repo.
func (h *Hub) SocketPath() string { return SocketPath(h.root) }

// NewAgent registers an agent identity (no pod). Identity precedes runtime (D13).
func (h *Hub) NewAgent(name, role string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid agent name %q (use lowercase letters, digits, - _)", name)
	}
	if role != "worker" && role != "reviewer" {
		return fmt.Errorf("invalid role %q (worker|reviewer)", role)
	}
	if _, ok, err := h.store.GetAgent(name); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("agent %q already exists", name)
	}
	a := store.Agent{
		Name:      name,
		Role:      role,
		Workspace: filepath.Join(".worktrees", name),
		Socket:    filepath.Join(".sindri", "sockets", name+".sock"),
	}
	if err := h.store.PutAgent(a); err != nil {
		return err
	}
	return h.store.Log(name, "register", "role="+role)
}

// Launch spins a pod that assumes an existing agent's identity. The agent's
// workspace worktree is created on demand; the pod runs interactive Claude in a
// tmux session named after the agent.
func (h *Hub) Launch(name string) error {
	a, ok, err := h.store.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q — run 'sindri new %s' first", name, name)
	}
	if err := container.Ensure(h.root); err != nil {
		return err
	}
	wt := filepath.Join(h.root, a.Workspace)
	if !git.HasCommits(h.root) {
		return fmt.Errorf("repo has no commits yet")
	}
	if err := git.WorktreeAdd(h.root, wt, "HEAD"); err != nil {
		return err
	}
	cName := Container(name)
	_ = pod.Rm(cName) // clear any stale container with this name

	opts := pod.RunOpts{
		Name:   cName,
		Image:  container.ImageName,
		Labels: map[string]string{"sindri.project": h.root, "sindri.agent": name},
		Env:    map[string]string{"SINDRI_AGENT": name, "COLORTERM": "truecolor"},
		Mounts: []pod.Mount{
			{Host: wt, Container: "/workspace", Mode: "rw"},
			// Phase 2 adds the agent's own socket mount (identity-by-socket); the
			// Phase 1 agent has no browser client, so it needs only its workspace.
		},
		Workdir:    "/workspace",
		Entrypoint: []string{"sindri-agent", name},
	}
	if err := pod.Run(opts); err != nil {
		return err
	}
	return h.store.Log(name, "launch", "container="+cName)
}

// Tell delivers a message into an agent's session, stamped with its source
// (provenance, D12). The stamped line is recorded in the activity log.
func (h *Hub) Tell(name, msg, source string) error {
	a, ok, err := h.store.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if source == "" {
		source = "user"
	}
	stamped := fmt.Sprintf("[%s] %s", source, msg)
	if err := h.inject(name, stamped); err != nil {
		return err
	}
	_ = a
	return h.store.Log(name, "recv", stamped)
}

// inject types text into an agent's tmux session via podman exec.
func (h *Hub) inject(name, text string) error {
	c := Container(name)
	if !pod.Running(c) {
		return fmt.Errorf("agent %q is not running — launch it first", name)
	}
	for _, argv := range tmux.SendText(session(name), text) {
		full := append([]string{"tmux"}, argv...)
		if _, err := pod.Exec(c, full...); err != nil {
			return err
		}
	}
	return nil
}

// State returns every agent with its live running status.
func (h *Hub) State() ([]AgentState, error) {
	roster, err := h.store.Roster()
	if err != nil {
		return nil, err
	}
	out := make([]AgentState, 0, len(roster))
	for _, a := range roster {
		out = append(out, AgentState{Agent: a, Running: pod.Running(Container(a.Name))})
	}
	return out, nil
}
