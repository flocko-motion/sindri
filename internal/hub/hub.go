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
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// Hub is the per-repo coordinator. It is the only writer of the store and the
// only thing that drives pods/tmux.
type Hub struct {
	root  string
	store *store.Store

	mu      sync.Mutex              // guards agentLn
	agentLn map[string]net.Listener // per-agent socket listeners (identity-by-socket)
	events  *bus                    // change notifications for /events

	lcMu      sync.Mutex        // guards lifecycle
	lifecycle map[string]string // transient launch/stop intent: name -> "launching"|"stopping"

	launchMu  sync.Mutex             // guards launchBuf
	launchBuf map[string]*safeBuffer // per-agent image-build/pod-start output
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

// container is Container bound to this hub's repo (the common in-hub case).
func (h *Hub) container(name string) string { return Container(h.root, name) }

// session is the tmux session name for an agent (named after the agent, D4).
func session(name string) string { return name }

// plannerBranch is a planner's standing branch — it drafts openspec here and
// ships it via `openspec submit` (it never grabs a backlog task).
func plannerBranch(name string) string { return "plan-" + name }

// mockSpecTask is the placeholder todo id on a planner's openspec PR (there's no
// real backlog task behind it).
const mockSpecTask = "os-new"

// restPhase is an agent's resting (not-busy) phase: a planner rests in "planning"
// and a coauthor in "collab" (neither holds a backlog task, so "idle" would
// mislead — they're standing with the user, not unoccupied); everyone else "idle".
func restPhase(role string) string {
	switch role {
	case "planner":
		return "planning"
	case "coauthor":
		return "collab"
	default:
		return "idle"
	}
}

// New opens the hub for a project: ensures `.sindri/` exists and opens the DB.
func New(root string) (*Hub, error) {
	dir := filepath.Join(root, ".sindri")
	if err := os.MkdirAll(filepath.Join(dir, "sockets"), 0o755); err != nil {
		return nil, fmt.Errorf("create .sindri: %w", err)
	}
	ensureGitignore(root) // keep the hub's process artifacts out of the repo
	st, err := store.Open(filepath.Join(dir, "hub.db"))
	if err != nil {
		return nil, err
	}
	return &Hub{root: root, store: st, agentLn: map[string]net.Listener{}, events: newBus(),
		lifecycle: map[string]string{}, launchBuf: map[string]*safeBuffer{}}, nil
}

// hubIgnores are the hub's process artifacts that must never be committed: its
// own state/home dir and the agent worktrees (per-agent Claude homes carry
// history, backups, and snapshots — a lot of churn). `.todos/` is deliberately
// NOT ignored: task data is tracked.
var hubIgnores = []string{".sindri/", ".worktrees/"}

// ensureGitignore appends any missing hub-artifact patterns to the repo's
// .gitignore (creating it if absent), idempotently — so a fresh project never
// fills lazygit/`git status` with hub churn. Best-effort and loud on failure: it
// never blocks hub startup, but a write error is reported rather than swallowed.
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
func (h *Hub) setLifecycle(name, state string) {
	h.lcMu.Lock()
	defer h.lcMu.Unlock()
	if state == "" {
		delete(h.lifecycle, name)
	} else {
		h.lifecycle[name] = state
	}
}

// agentStatus reconciles transient intent with observed runtime into one status
// word — and clears the intent once fulfilled (launching→running, stopping→
// down). The single source of truth for "what is this agent doing".
func (h *Hub) agentStatus(name string, running bool, phase string) string {
	h.lcMu.Lock()
	defer h.lcMu.Unlock()
	intent := h.lifecycle[name]
	switch {
	case intent == "stopping":
		if running {
			return "stopping" // stop requested, pod still up
		}
		delete(h.lifecycle, name) // down now — stop intent fulfilled
		return "down"
	case running:
		delete(h.lifecycle, name) // up now — launch intent fulfilled
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
	return h.store.Close()
}

// SocketPath is the hub's control socket for this repo.
func (h *Hub) SocketPath() string { return SocketPath(h.root) }

// NewAgent registers an agent identity (no pod). Identity precedes runtime
// (D13). An empty name is auto-assigned a Norse dwarf name. Returns the final
// name.
func (h *Hub) NewAgent(name, role string) (string, error) {
	if name == "" { // auto-name after a dwarf — a friend of Sindri
		n, err := h.autoName()
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
	if _, ok, err := h.store.GetAgent(name); err != nil {
		return "", err
	} else if ok {
		return "", fmt.Errorf("agent %q already exists", name)
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
		Socket:    filepath.Join(".sindri", "sockets", name+".sock"),
	}
	if err := h.store.PutAgent(a); err != nil {
		return "", err
	}
	defer h.notify()
	return name, h.store.Log(name, "register", "role="+role)
}

// DeleteAgent removes an agent entirely: stops its pod, closes its socket
// listener, removes its worktree, and drops its identity (and activity log)
// from the roster. Best-effort on the runtime teardown — a missing pod or
// worktree is fine; the identity is always removed.
func (h *Hub) DeleteAgent(name string) error {
	a, ok, err := h.store.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	// Release the agent's task back to the backlog so it isn't stranded
	// in_progress with no owner. (A planner's os-new sentinel and openspec items
	// aren't real td tasks — skip those.)
	if st, _ := h.store.GetState(name); strings.HasPrefix(st.Task, "td-") {
		if err := td.SetStatus(h.root, st.Task, "open"); err != nil {
			fmt.Printf("warning: reopen %s on delete of %s: %v\n", st.Task, name, err)
		}
		_ = h.refreshTask(st.Task)
	}
	_ = pod.Rm(h.container(name))
	h.closeAgent(name)
	// A coauthor's workspace is the repo root itself (the shared checkout), not a
	// disposable worktree — never run `git worktree remove` on it.
	if a.Workspace != "." {
		_ = git.WorktreeRemove(h.root, filepath.Join(h.root, a.Workspace))
	}
	if err := h.store.DeleteAgent(name); err != nil {
		return err
	}
	h.notify()
	return nil
}

// StopAgent is the opposite of Launch: it tears down the agent's pod (killing
// its tmux session) but keeps the identity, worktree, socket listener, and
// activity log — so it can be re-launched and resumes where it left off.
func (h *Hub) StopAgent(name string) error {
	if _, ok, err := h.store.GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if !pod.Running(h.container(name)) {
		return fmt.Errorf("agent %q is not running", name)
	}
	h.setLifecycle(name, "stopping") // status → stopping (pod still up); → down once gone
	h.notify()
	if err := pod.Rm(h.container(name)); err != nil {
		h.setLifecycle(name, "")
		h.notify()
		return err
	}
	_ = h.store.Log(name, "stop", "pod removed")
	h.notify()
	return nil
}

// Launch spins a pod that assumes an existing agent's identity. The agent's
// workspace worktree is created on demand; the pod runs interactive Claude in a
// tmux session named after the agent (or a bare shell when shell is true — used
// for deterministic demos and debugging).
func (h *Hub) Launch(name string, shell bool) (err error) {
	a, ok, err := h.store.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q — run 'sindri new %s' first", name, name)
	}
	// Status → launching immediately (cleared by State once the pod is up); on
	// any failure below, clear it so it doesn't stick at "launching".
	h.setLifecycle(name, "launching")
	_ = h.store.Log(name, "launch", "requested")
	h.notify()
	defer func() {
		if err != nil {
			h.setLifecycle(name, "")
			h.notify()
		}
	}()
	// Tee the image build (+ our progress notes) into the agent's launch buffer
	// so the TUI live-screen region shows it while the pod comes up.
	buf := h.newLaunchBuf(name)
	if err := container.Ensure(h.root, io.MultiWriter(os.Stderr, buf)); err != nil {
		return err
	}
	fmt.Fprintf(buf, "Image ready. Starting pod %s…\n", h.container(name))
	wt := filepath.Join(h.root, a.Workspace)
	if !git.HasCommits(h.root) {
		return fmt.Errorf("repo has no commits yet")
	}
	if a.Role == "coauthor" {
		// A coauthor's /workspace IS the user's checkout (wt == repo root) — no
		// isolated worktree to add. Rest in "collab" so the dashboard shows it's
		// standing with the user, not idle.
		if st, _ := h.store.GetState(name); st.Phase == "" || st.Phase == "idle" {
			_ = h.store.SetState(store.AgentState{Agent: name, Phase: "collab"})
		}
	} else if err := git.WorktreeAdd(h.root, wt, "HEAD"); err != nil {
		return err
	}
	if a.Role == "planner" {
		// Put the planner on its standing branch so it can draft openspec and ship
		// it via `openspec submit` without ever grabbing a backlog task.
		base, err := h.baseBranch()
		if err != nil {
			return err
		}
		if err := git.EnsureBranch(wt, plannerBranch(name), base); err != nil {
			return err
		}
		// Rest in "planning", not "idle" — unless a PR is already in flight.
		if st, _ := h.store.GetState(name); st.Phase != "submitted" {
			_ = h.store.SetState(store.AgentState{Agent: name, Phase: "planning"})
		}
	}
	// Serve the agent's own socket BEFORE the pod launches — the pod bind-mounts
	// it, and the socket IS the agent's identity (D2). Requires the persistent
	// hub: an ephemeral in-process hub would take the listener down on exit.
	if err := h.ServeAgent(name); err != nil {
		return err
	}
	workerBin, err := agentBinary()
	if err != nil {
		return err
	}
	cName := h.container(name)
	_ = pod.Rm(cName) // clear any stale container with this name

	env := map[string]string{"SINDRI_AGENT": name, "COLORTERM": "truecolor"}
	mounts := []pod.Mount{
		{Host: wt, Container: "/workspace", Mode: "rw"},
		// The agent's own socket — its sole channel to the hub, its identity.
		// Mount the socket DIRECTORY (not the file) so the agent survives a hub
		// restart, which recreates the socket file with a new inode.
		{Host: AgentSocketDir(h.root, name), Container: "/run/sindri", Mode: "rw"},
		// The thin browser binary (image symlinks it to /usr/local/bin/sindri — the
		// agent's in-pod interface to the hub).
		{Host: workerBin, Container: "/opt/sindri/sindri-worker", Mode: "ro"},
	}
	if a.Role == "planner" {
		// A planner sees the whole repo read-only and may only write openspec — so
		// it plans (specs + tasks) without touching code. /workspace is remounted
		// ro and openspec/ overlaid rw on top.
		osDir := filepath.Join(wt, "openspec")
		_ = os.MkdirAll(osDir, 0o755) // ensure the overlay target exists
		mounts[0] = pod.Mount{Host: wt, Container: "/workspace", Mode: "ro"}
		mounts = append(mounts, pod.Mount{Host: osDir, Container: "/workspace/openspec", Mode: "rw"})
	}
	if a.Role == "coauthor" {
		// The coauthor's /workspace IS the user's checkout, so it would otherwise see
		// .sindri/ (hub.db, sockets) live. Overlay an empty read-only dir over
		// /workspace/.sindri so it can't read or corrupt hub state while it works.
		shield := filepath.Join(h.root, ".sindri", "shield")
		if err := os.MkdirAll(shield, 0o755); err != nil {
			return err
		}
		mounts = append(mounts, pod.Mount{Host: shield, Container: "/workspace/.sindri", Mode: "ro"})
	}
	if shell {
		env["SINDRI_SHELL"] = "1" // entrypoint runs bash instead of Claude
	} else {
		// Set up the agent's Claude home (credentials, config, system prompt) and
		// mount it so Claude runs authenticated.
		home, cfg, hasCreds, err := h.prepareClaudeHome(name, a.Role)
		if err != nil {
			return err
		}
		if !hasCreds {
			return fmt.Errorf("no Claude credentials on host (~/.claude/.credentials.json) — launch with --shell, or log in")
		}
		mounts = append(mounts,
			pod.Mount{Host: home, Container: "/home/sindri/.claude", Mode: "rw"},
			pod.Mount{Host: cfg, Container: "/home/sindri/.claude.json", Mode: "rw"})
		// Mount the user's Claude skills into the agent's home so it works with the
		// same skills the user has — read-only and live (edits on the host show up
		// without a relaunch). Any symlinks inside are the user's to manage.
		if host, herr := os.UserHomeDir(); herr == nil {
			skills := filepath.Join(host, ".claude", "skills")
			if fi, serr := os.Stat(skills); serr == nil && fi.IsDir() {
				mounts = append(mounts, pod.Mount{Host: skills, Container: "/home/sindri/.claude/skills", Mode: "ro"})
			}
		}
	}
	opts := pod.RunOpts{
		Name:       cName,
		Image:      container.ImageName,
		Labels:     map[string]string{"sindri.project": h.root, "sindri.agent": name},
		Env:        env,
		Mounts:     mounts,
		Workdir:    "/workspace",
		Entrypoint: []string{"sindri-agent", name},
	}
	if err := pod.Run(opts); err != nil {
		return err
	}
	if err := h.store.Log(name, "launch", "started container="+cName); err != nil {
		return err
	}
	go h.rehydrate(name) // resume from the activity log once the session is up (D13)
	h.notify()
	return nil
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
	defer h.notify()
	return h.store.Log(name, "recv", stamped)
}

// inject types text into an agent's tmux session via podman exec.
func (h *Hub) inject(name, text string) error {
	c := h.container(name)
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

// injectWhenReady waits (briefly) for an agent's tmux session to exist, then
// injects. Used for hub-originated messages (verdicts, rehydrate) right after a
// launch, when the session may not be up yet. A message that never lands is
// recorded so it is not silently lost.
func (h *Hub) injectWhenReady(name, text string) error {
	c := h.container(name)
	for i := 0; i < 25; i++ {
		if pod.Running(c) {
			if _, err := pod.Exec(c, "tmux", "has-session", "-t", session(name)); err == nil {
				return h.inject(name, text)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return h.store.Log(name, "inject-skipped", text)
}

// rehydrate injects a kickoff/briefing once a (re)launched pod's session is up
// (D13). A fresh agent gets a role-appropriate nudge; a resuming one gets the
// tail of its activity log so it can pick up where it left off. Best-effort;
// runs in the background so it doesn't block launch.
// resumeEvents are the activity types worth replaying to an agent on resume —
// its actual work, not pod lifecycle or injected chatter.
var resumeEvents = map[string]bool{
	"claim": true, "submit": true, "note": true,
	"approve": true, "reject": true, "merged": true, "lint-fail": true,
	"recv": true,
}

func (h *Hub) rehydrate(name string) {
	evs, _ := h.store.Events(name, 40)
	// Summarize only the agent's own work — not pod lifecycle (launch/stop/
	// register) or injected messages — so resume context is signal, not noise.
	var recent []string
	for _, e := range evs {
		if resumeEvents[e.Type] {
			recent = append(recent, e.Type+" "+e.Payload)
		}
	}
	var msg string
	if len(recent) == 0 { // no work yet — a fresh kickoff
		msg = msgKickoff
	} else {
		if len(recent) > 5 { // just the last few
			recent = recent[len(recent)-5:]
		}
		msg = msgResuming(strings.Join(recent, " · "))
	}
	// Let the agent program (Claude) boot to input-readiness before the kickoff,
	// or its submitting Enter is eaten by the boot splash.
	time.Sleep(8 * time.Second)
	_ = h.injectWhenReady(name, msg)
}

// agentBinary locates the thin browser binary on the host: next to the running
// sindri binary first, then on PATH.
func agentBinary() (string, error) {
	const name = "sindri-worker"
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s binary not found — run 'make build/install'", name)
}

// brokkrBinary locates the brokkr toolbelt binary (which runs the linters): next
// to the running sindri binary first, then on PATH. The lint gate shells out to
// it (brokkr ships alongside sindri).
func brokkrBinary() (string, error) {
	const name = "brokkr"
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("brokkr binary not found — it ships with sindri ('make install')")
}

