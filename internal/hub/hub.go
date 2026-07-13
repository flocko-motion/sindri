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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/tasks/td"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/chat"
	"github.com/flo-at/sindri/internal/hub/comments"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// Hub is the single global coordinator across every repo. It is the only writer
// of the store and the only thing that drives pods/tmux. Per-repo work is scoped
// by an agentKey (project + name); repos are resolved to a store handle via
// store.For(repoTag).
type Hub struct {
	store *store.Store

	mu      sync.Mutex                // guards agentLn
	agentLn map[agentKey]net.Listener // per-agent socket listeners (Linux identity-by-socket)
	events  *bus                      // change notifications for /events

	// macOS only: a bind-mounted unix socket can't be connected to across the
	// podman VM boundary, so agents reach the hub over a loopback TCP channel
	// authenticated by a per-agent token. Zero/nil on Linux (unix sockets suffice).
	agentTCPLn    net.Listener
	agentTCPPort  int
	agentDialHost string // how a pod addresses the host for the TCP channel (runtime-specific)

	lcMu      sync.Mutex          // guards lifecycle
	lifecycle map[agentKey]string // transient launch/stop intent: "launching"|"stopping"

	launchMu  sync.Mutex               // guards launchBuf
	launchBuf map[agentKey]*safeBuffer // per-agent image-build/pod-start output

	chat     *chat.Service     // the user's chatroom relay (internal/hub/chat)
	comments *comments.Service // task-comment sync (internal/hub/comments)
	agents   *agent.Service    // agent identity/auth/memory (internal/hub/agent)
	wf       *workflow.Engine  // the PR/task lifecycle orchestrator (internal/hub/workflow)
}

// agentKey identifies an agent within a project — the key for the hub's per-agent
// maps now that one hub serves many repos. project is a repoTag.
type agentKey struct {
	project string
	name    string
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

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

// session is the tmux session name for an agent (named after the agent, D4).
func session(name string) string { return name }

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
	h := &Hub{store: st, events: newBus(),
		agentLn:   map[agentKey]net.Listener{},
		lifecycle: map[agentKey]string{},
		launchBuf: map[agentKey]*safeBuffer{}}
	h.chat = chat.New(h.store, chatDelivery{h})
	h.comments = comments.New(h.store, commentsDeps{h})
	h.agents = agent.New(h.store, h.notify)
	h.wf = workflow.New(h.store, workflowDeps{h})
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

// setLifecycle records a transient launch/stop intent for an agent (cleared by
// State once observed reality catches up). "" clears it.
func (h *Hub) setLifecycle(project, name, state string) {
	h.lcMu.Lock()
	defer h.lcMu.Unlock()
	key := agentKey{project, name}
	if state == "" {
		delete(h.lifecycle, key)
	} else {
		h.lifecycle[key] = state
	}
}

// agentStatus reconciles transient intent with observed runtime into one status
// word — and clears the intent once fulfilled (launching→running, stopping→
// down). The single source of truth for "what is this agent doing".
func (h *Hub) agentStatus(project, name string, running bool, phase string) string {
	h.lcMu.Lock()
	defer h.lcMu.Unlock()
	key := agentKey{project, name}
	intent := h.lifecycle[key]
	switch {
	case intent == "stopping":
		if running {
			return "stopping" // stop requested, pod still up
		}
		delete(h.lifecycle, key) // down now — stop intent fulfilled
		return "down"
	case running:
		delete(h.lifecycle, key) // up now — launch intent fulfilled
		if phase == "" {
			return "idle"
		}
		return phase
	case intent == "launching":
		return "launching" // requested, pod not up yet
	default:
		return "down"
	}
}

// Close shuts agent listeners and releases the store.
func (h *Hub) Close() error {
	h.closeAgents()
	if h.agentTCPLn != nil {
		h.agentTCPLn.Close()
	}
	accessLogger.Flush() // emit any open access-log run before we go quiet
	return h.store.Close()
}

// SocketPath is the global hub's control socket.
func (h *Hub) SocketPath() string { return SocketPath() }

// NewAgent registers an agent identity in a project (no pod). Identity precedes
// runtime (D13). An empty name is auto-assigned a Norse dwarf name unused in that
// project. Returns the final name.
func (h *Hub) NewAgent(project, name, role, memory string) (string, error) {
	ps := h.store.For(project)
	if name == "" { // auto-name after a dwarf — a friend of Sindri (globally unique)
		n, err := h.agents.AutoName()
		if err != nil {
			return "", err
		}
		name = n
	}
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("invalid agent name %q (use lowercase letters, digits, - _)", name)
	}
	if role != "worker" && role != "reviewer" && role != "planner" && role != "coauthor" {
		return "", fmt.Errorf("invalid role %q (worker|reviewer|planner|coauthor)", role)
	}
	if !agent.ValidMemory(memory) {
		return "", fmt.Errorf("invalid memory %q (e.g. 2g, 512m)", memory)
	}
	// Names are unique across ALL repos — a dwarf identifies one agent machine-wide,
	// so the unified board never shows two agents with the same name.
	agents, err := h.store.AllAgents()
	if err != nil {
		return "", err
	}
	for _, a := range agents {
		if a.Name == name {
			return "", fmt.Errorf("agent %q already exists (in %s) — names are unique across all repos", name, a.Project)
		}
	}
	// A coauthor shares the user's real checkout (the repo root) rather than an
	// isolated worktree — it works the SAME material as the user, freestyle.
	workspace := filepath.Join(".worktrees", name)
	if role == "coauthor" {
		workspace = "."
	}
	a := store.Agent{
		Name:      name,
		Role:      role,
		Workspace: workspace,
		Socket:    filepath.Join(AgentSocketDir(project, name), "sock"),
		Memory:    strings.TrimSpace(memory),
	}
	if err := ps.PutAgent(a); err != nil {
		return "", err
	}
	defer h.notify()
	return name, ps.Log(name, "register", "role="+role)
}

// DeleteAgent removes an agent entirely: stops its pod, closes its socket
// listener, removes its worktree, and drops its identity (and activity log)
// from the roster. Best-effort on the runtime teardown — a missing pod or
// worktree is fine; the identity is always removed.
func (h *Hub) DeleteAgent(project, name string) error {
	ps := h.store.For(project)
	root := h.projectRoot(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	// Release the agent's task back to the backlog so it isn't stranded
	// in_progress with no owner. (A planner's os-new sentinel and openspec items
	// aren't real td tasks — skip those.)
	if st, _ := ps.GetState(name); strings.HasPrefix(st.Task, "td-") {
		if err := td.SetStatus(root, st.Task, "open"); err != nil {
			fmt.Printf("warning: reopen %s on delete of %s: %v\n", st.Task, name, err)
		}
		_ = h.wf.RefreshTask(project, st.Task)
	}
	_ = container.Rm(h.container(project, name))
	h.closeAgent(project, name)
	// A coauthor's workspace is the repo root itself (the shared checkout), not a
	// disposable worktree — never run `git worktree remove` on it.
	if a.Workspace != "." {
		_ = git.WorktreeRemove(root, filepath.Join(root, a.Workspace))
	}
	if err := ps.DeleteAgent(name); err != nil {
		return err
	}
	h.notify()
	return nil
}

// StopAgent is the opposite of Launch: it tears down the agent's pod (killing
// its tmux session) but keeps the identity, worktree, socket listener, and
// activity log — so it can be re-launched and resumes where it left off.
func (h *Hub) StopAgent(project, name string) error {
	ps := h.store.For(project)
	if _, ok, err := ps.GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if !container.Running(h.container(project, name)) {
		return fmt.Errorf("agent %q is not running", name)
	}
	h.setLifecycle(project, name, "stopping") // status → stopping (pod up); → down once gone
	h.notify()
	if err := container.Rm(h.container(project, name)); err != nil {
		h.setLifecycle(project, name, "")
		h.notify()
		return err
	}
	_ = ps.Log(name, "stop", "pod removed")
	h.notify()
	return nil
}

// Launch spins a pod that assumes an existing agent's identity. The agent's
// workspace worktree is created on demand; the pod runs interactive Claude in a
// tmux session named after the agent (or a bare shell when shell is true — used
// for deterministic demos and debugging).
func (h *Hub) Launch(project, name string, shell, debug bool, progress io.Writer) (err error) {
	ps := h.store.For(project)
	root := h.projectRoot(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q — run 'sindri new %s' first", name, name)
	}
	// Validate the project config up front — a bad .sindri/config.yaml fails the
	// launch loudly rather than silently reverting to defaults mid-build.
	cfg, err := h.projectConfig(project)
	if err != nil {
		return err
	}
	// Tee build/start progress three ways: the launch buffer (TUI live-screen), the
	// hub log (stderr), and progress — the caller's stream, so `agent start` shows
	// the image build live instead of a frozen prompt (long ops must be visible).
	buf := h.newLaunchBuf(project, name)
	w := io.MultiWriter(os.Stderr, buf, progress)
	// Pre-flight: podman must be installed and reachable. Fail fast with an
	// actionable message (before touching status or staging an image build) rather
	// than surfacing a cryptic exit code mid-build. On macOS/Windows this also
	// auto-starts a stopped podman VM, teeing that progress into the launch buffer.
	if err := container.Check(w); err != nil {
		return err
	}
	// Status → launching immediately (cleared by State once the pod is up); on
	// any failure below, clear it so it doesn't stick at "launching".
	h.setLifecycle(project, name, "launching")
	_ = ps.Log(name, "launch", "requested")
	h.notify()
	defer func() {
		if err != nil {
			h.setLifecycle(project, name, "")
			h.notify()
		}
	}()
	imageRef, err := container.EnsureImage(root, config.Abs(root, cfg.Containerfile), w)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Image ready. Starting container %s…\n", h.container(project, name))
	wt := filepath.Join(root, a.Workspace)
	hasCommits, err := git.HasCommits(root)
	if err != nil {
		return err
	}
	if !hasCommits {
		return fmt.Errorf("repo has no commits yet")
	}
	if a.Role == "coauthor" {
		// A coauthor's /workspace IS the user's checkout (wt == repo root) — no
		// isolated worktree to add. Rest in "collab" so the dashboard shows it's
		// standing with the user, not idle.
		if st, _ := ps.GetState(name); st.Phase == "" || st.Phase == "idle" {
			_ = ps.SetState(store.AgentState{Agent: name, Phase: "collab"})
		}
	} else if err := git.WorktreeAdd(root, wt, "HEAD"); err != nil {
		return err
	}
	if a.Role == "planner" {
		// Put the planner on its standing branch so it can draft openspec and ship
		// it via `openspec submit` without ever grabbing a backlog task.
		base, err := git.CurrentBranch(root)
		if err != nil {
			return err
		}
		if err := git.EnsureBranch(wt, workflow.PlannerBranch(name), base); err != nil {
			return err
		}
		// Rest in "planning", not "idle" — unless a PR is already in flight.
		if st, _ := ps.GetState(name); st.Phase != "submitted" {
			_ = ps.SetState(store.AgentState{Agent: name, Phase: "planning"})
		}
	}
	// Serve the agent's own socket BEFORE the pod launches — the pod bind-mounts it,
	// and the socket IS the agent's identity (D2); needs the persistent hub.
	if err := h.ServeAgent(project, name); err != nil {
		return err
	}
	workerBin, err := agent.Binary()
	if err != nil {
		return err
	}
	cName := h.container(project, name)
	_ = container.Rm(cName) // clear any stale container with this name

	env := map[string]string{"SINDRI_AGENT": name, "COLORTERM": "truecolor"}
	// macOS: the pod can't connect to the bind-mounted unix socket across the VM
	// boundary, so point the worker at the loopback TCP channel with its token. On
	// Linux these are unset and the worker uses /run/sindri/sock (below).
	if runtime.GOOS == "darwin" {
		if h.agentTCPPort == 0 {
			return fmt.Errorf("agent TCP channel not started — launch needs a persistent hub")
		}
		token, terr := h.agents.Token(project, name)
		if terr != nil {
			return terr
		}
		env["SINDRI_HUB_ADDR"] = fmt.Sprintf("%s:%d", h.agentDialHost, h.agentTCPPort)
		env["SINDRI_TOKEN"] = token
	}
	mounts := []container.Mount{
		{Host: wt, Container: "/workspace", Mode: "rw"},
		// The agent's own socket — its sole channel to the hub, its identity.
		// Mount the socket DIRECTORY (not the file) so the agent survives a hub
		// restart, which recreates the socket file with a new inode.
		{Host: AgentSocketDir(project, name), Container: "/run/sindri", Mode: "rw"},
		// The thin browser binary (image symlinks it to /usr/local/bin/sindri — the
		// agent's in-pod interface to the hub).
		{Host: workerBin, Container: "/opt/sindri/sindri-worker", Mode: "ro"},
	}
	// Low-friction parity: mount brokkr into every pod so the SAME `brokkr` commands
	// (map, lint) work inside the agent regardless of host OS. Pods are always linux,
	// so we mount a cross-built linux brokkr (agent.BrokkrLinuxBinary): on a linux host that
	// is the host binary itself; on macOS it's the shipped brokkr-linux. Runtime mount,
	// so a restart picks it up — no image rebuild.
	if bk, berr := agent.BrokkrLinuxBinary(); berr == nil {
		mounts = append(mounts, container.Mount{Host: bk, Container: "/usr/local/bin/brokkr", Mode: "ro"})
	}
	if a.Role == "planner" {
		// A planner sees the whole repo read-only and may only write openspec — so
		// it plans (specs + tasks) without touching code. /workspace is remounted
		// ro and openspec/ overlaid rw on top.
		osDir := filepath.Join(wt, "openspec")
		_ = os.MkdirAll(osDir, 0o755) // ensure the overlay target exists
		mounts[0] = container.Mount{Host: wt, Container: "/workspace", Mode: "ro"}
		mounts = append(mounts, container.Mount{Host: osDir, Container: "/workspace/openspec", Mode: "rw"})
	}
	// Hub state lives centrally, never in the repo — a coauthor's shared checkout needs no shield.
	if shell {
		env["SINDRI_SHELL"] = "1" // entrypoint runs bash instead of Claude
	} else {
		// Compose the agent's system prompt (workflow logic: identity + how-to-work,
		// with the project architecture injected), then hand it to the coding-agent
		// backend to provision its home (credentials, config, prompt) — the hub owns
		// only WHERE that home lives.
		archPath := h.architectureDoc(project)
		archContent, _ := os.ReadFile(filepath.Join(h.projectRoot(project), archPath))
		sysPrompt := workflow.SystemPrompt(name, a.Role, string(archContent), archPath)
		homeDir := filepath.Join(paths.StateDir(), project, "agents", name)
		home, err := agentport.PrepareHome(agentport.HomeSpec{Dir: homeDir, SystemPrompt: sysPrompt, Out: w})
		if err != nil {
			return err
		}
		if !home.HasCreds {
			return fmt.Errorf("no Claude credentials on host (~/.claude/.credentials.json, or the macOS Keychain) — log in with `claude`, or launch with --shell")
		}
		mounts = append(mounts,
			container.Mount{Host: home.Dir, Container: "/home/sindri/.claude", Mode: "rw"},
			container.Mount{Host: home.ConfigPath, Container: "/home/sindri/.claude.json", Mode: "rw"})
		// Mount the user's Claude skills into the agent's home so it works with the
		// same skills the user has — read-only and live (edits on the host show up
		// without a relaunch). Any symlinks inside are the user's to manage.
		if host, herr := os.UserHomeDir(); herr == nil {
			skills := filepath.Join(host, ".claude", "skills")
			if fi, serr := os.Stat(skills); serr == nil && fi.IsDir() {
				mounts = append(mounts, container.Mount{Host: skills, Container: "/home/sindri/.claude/skills", Mode: "ro"})
			}
		}
	}
	opts := container.RunOpts{
		Name:       cName,
		Image:      imageRef,
		Labels:     map[string]string{"sindri.project": root, "sindri.agent": name},
		Env:        env,
		Mounts:     mounts,
		Workdir:    "/workspace",
		Entrypoint: []string{"sindri-agent", name},
		Memory:     agent.MemoryOrDefault(a.Memory),
	}
	if err := container.Run(opts); err != nil {
		return err
	}
	if err := ps.Log(name, "launch", "started container="+cName); err != nil {
		return err
	}
	// Stay until we OBSERVE the agent up (container running AND tmux session answers)
	// — no optimistic "launched" while it's still coming up. On timeout report the
	// failure (deferred cleanup clears the launching intent → board shows "down").
	fmt.Fprintf(w, "Waiting for %s to come up…\n", name)
	deadline := time.Now().Add(launchReadyTimeout)
	shown := 0
	for !h.agentAlive(project, name) {
		if full := container.Logs(cName, 1000); len(full) > shown { // follow the container's output during the wait
			fmt.Fprint(w, full[shown:])
			shown = len(full)
		}
		if debug { // --debug: surface what the hub's liveness probe actually observes
			fmt.Fprintf(w, "  [debug] %s\n", container.Diagnose(context.Background(), cName))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s launched but didn't come up within %s: %s (check `sindri agent pane %s`)",
				name, launchReadyTimeout, h.launchDiagnostic(project, name), name)
		}
		time.Sleep(time.Second)
	}
	h.setLifecycle(project, name, "") // observed up — clear the launching intent now
	fmt.Fprintf(w, "Agent %s is up.\n", name)
	go h.rehydrate(project, name) // nudge it to resume once the session is live (D13)
	h.notify()
	return nil
}

// launchReadyTimeout bounds how long Launch waits to observe a freshly-launched
// agent's session come up before reporting the launch failed. Generous: a cold
// micro-VM/pod boot plus the entrypoint starting tmux can take a bit.
const launchReadyTimeout = 45 * time.Second

// Tell delivers a message into an agent's session, stamped with its source
// (provenance, D12). The stamped line is recorded in the activity log.
func (h *Hub) Tell(project, name, msg, source string) error {
	ps := h.store.For(project)
	if _, ok, err := ps.GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if source == "" {
		source = "user"
	}
	stamped := fmt.Sprintf("[%s] %s", source, msg)
	if err := h.inject(project, name, stamped); err != nil {
		return err
	}
	defer h.notify()
	return ps.Log(name, "recv", stamped)
}

// inject types text into an agent's tmux session via podman exec.
func (h *Hub) inject(project, name, text string) error {
	c := h.container(project, name)
	if !container.Running(c) {
		return fmt.Errorf("agent %q is not running — launch it first", name)
	}
	for _, argv := range tmux.SendText(session(name), text) {
		full := append([]string{"tmux"}, argv...)
		if _, err := container.Exec(c, full...); err != nil {
			return err
		}
	}
	return nil
}

// injectWhenReady waits (briefly) for an agent's tmux session to exist, then
// injects. Used for hub-originated messages (verdicts, rehydrate) right after a
// launch, when the session may not be up yet. A message that never lands is
// recorded so it is not silently lost.
func (h *Hub) injectWhenReady(project, name, text string) error {
	c := h.container(project, name)
	for i := 0; i < 25; i++ {
		if container.Running(c) {
			if _, err := container.Exec(c, "tmux", "has-session", "-t", session(name)); err == nil {
				return h.inject(project, name, text)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return h.store.For(project).Log(name, "inject-skipped", text)
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
	_ = h.injectWhenReady(project, name, workflow.MsgKickoff)
	// A chatroom member that just relaunched has lost the membership cue from its
	// durable prompt — remind it so it knows it can still talk to the room. Best-
	// effort: a store hiccup here shouldn't derail the rehydrate.
	if member, err := h.chat.IsMember(project, name); err != nil {
		fmt.Fprintf(os.Stderr, "hub: chat membership check for %s/%s failed: %v\n", project, name, err)
	} else if member {
		_ = h.injectWhenReady(project, name, chat.MsgReminder)
	}
}
