// package: hub/agent / lifecycle
// type:    logic (the agent pod lifecycle)
// job:     the mechanics of managing an agent's pod — register (New), Launch a pod
//          that assumes its identity, Stop (keep identity), Delete (full teardown),
//          Rebuild (fresh image + relaunch), plus the transient launching/stopping
//          intent the board reconciles. Triggers come from outside; this does the work.
// limits:  git/container/tmux go through the adapters; the system prompt + branch
//          names come from workflow, the coding agent's home from adapter/agent, the
//          socket from agentchan. What to inject on rehydrate is the hub's (via Deps).
package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/tasks/td"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agentchan"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// lcKey keys the transient lifecycle-intent map by (project, name).
type lcKey struct{ project, name string }

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// launchReadyTimeout bounds how long Launch waits to observe a freshly-launched
// agent's session come up before reporting the launch failed. Generous: a cold
// micro-VM/pod boot plus the entrypoint starting tmux can take a bit.
const launchReadyTimeout = 45 * time.Second

// setLifecycle records a transient launch/stop intent for an agent (cleared by
// AgentStatus once observed reality catches up). "" clears it.
func (s *Service) setLifecycle(project, name, state string) {
	s.lcMu.Lock()
	defer s.lcMu.Unlock()
	key := lcKey{project, name}
	if state == "" {
		delete(s.lifecycle, key)
	} else {
		s.lifecycle[key] = state
	}
}

// AgentStatus reconciles transient intent with observed runtime into one status word
// — and clears the intent once fulfilled (launching→running, stopping→down). The
// single source of truth for "what is this agent doing"; the board calls it.
func (s *Service) AgentStatus(project, name string, running bool, phase string) string {
	s.lcMu.Lock()
	defer s.lcMu.Unlock()
	key := lcKey{project, name}
	intent := s.lifecycle[key]
	switch {
	case intent == "stopping":
		if running {
			return "stopping" // stop requested, pod still up
		}
		delete(s.lifecycle, key) // down now — stop intent fulfilled
		return "down"
	case running:
		delete(s.lifecycle, key) // up now — launch intent fulfilled
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

// NewAgent registers an agent identity in a project (no pod). Identity precedes
// runtime (D13). An empty name is auto-assigned a Norse dwarf name unused in that
// project. Returns the final name.
func (s *Service) NewAgent(project, name, role, memory string) (string, error) {
	ps := s.store.For(project)
	if name == "" { // auto-name after a dwarf — a friend of Sindri (globally unique)
		n, err := s.AutoName()
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
	if !ValidMemory(memory) {
		return "", fmt.Errorf("invalid memory %q (e.g. 2g, 512m)", memory)
	}
	// Names are unique across ALL repos — a dwarf identifies one agent machine-wide,
	// so the unified board never shows two agents with the same name.
	agents, err := s.store.AllAgents()
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
		Socket:    agentchan.SocketPath(project, name),
		Memory:    strings.TrimSpace(memory),
	}
	if err := ps.PutAgent(a); err != nil {
		return "", err
	}
	defer s.deps.Notify()
	return name, ps.Log(name, "register", "role="+role)
}

// DeleteAgent removes an agent entirely: stops its pod, closes its socket listener,
// removes its worktree, and drops its identity (and activity log) from the roster.
// Best-effort on the runtime teardown — a missing pod or worktree is fine; the
// identity is always removed.
func (s *Service) DeleteAgent(project, name string) error {
	ps := s.store.For(project)
	root := s.deps.ProjectRoot(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	// Release the agent's task back to the backlog so it isn't stranded in_progress
	// with no owner. (A planner's os-new sentinel and openspec items aren't real td
	// tasks — skip those.)
	if st, _ := ps.GetState(name); strings.HasPrefix(st.Task, "td-") {
		if err := td.SetStatus(root, st.Task, "open"); err != nil {
			fmt.Printf("warning: reopen %s on delete of %s: %v\n", st.Task, name, err)
		}
		_ = s.deps.RefreshTask(project, st.Task)
	}
	_ = container.Rm(s.deps.ContainerName(project, name))
	s.agentCh.CloseAgent(project, name)
	// A coauthor's workspace is the repo root itself (the shared checkout), not a
	// disposable worktree — never run `git worktree remove` on it.
	if a.Workspace != "." {
		_ = git.WorktreeRemove(root, filepath.Join(root, a.Workspace))
	}
	if err := ps.DeleteAgent(name); err != nil {
		return err
	}
	s.deps.Notify()
	return nil
}

// StopAgent is the opposite of Launch: it tears down the agent's pod (killing its
// tmux session) but keeps the identity, worktree, socket listener, and activity log —
// so it can be re-launched and resumes where it left off.
func (s *Service) StopAgent(project, name string) error {
	ps := s.store.For(project)
	if _, ok, err := ps.GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if !container.Running(s.deps.ContainerName(project, name)) {
		return fmt.Errorf("agent %q is not running", name)
	}
	s.setLifecycle(project, name, "stopping") // status → stopping (pod up); → down once gone
	s.deps.Notify()
	if err := container.Rm(s.deps.ContainerName(project, name)); err != nil {
		s.setLifecycle(project, name, "")
		s.deps.Notify()
		return err
	}
	_ = ps.Log(name, "stop", "pod removed")
	s.deps.Notify()
	return nil
}

// RebuildAgent rebuilds the agent's image (re-pull base) then relaunches it, streaming
// build/restart progress to w. A bad project config fails loudly before any build; a
// running agent is stopped first so it comes up on the fresh image.
func (s *Service) RebuildAgent(project, name string, w io.Writer) error {
	ps := s.store.For(project)
	root := s.deps.ProjectRoot(project)
	if _, ok, err := ps.GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	cfg, err := s.deps.ProjectConfig(project)
	if err != nil {
		return err
	}
	if _, err := container.RebuildImage(root, config.Abs(root, cfg.Containerfile), w); err != nil {
		return err
	}
	if container.Running(s.deps.ContainerName(project, name)) {
		fmt.Fprintf(w, "Image rebuilt — restarting %s to run it (the session resumes)…\n", name)
		if err := s.StopAgent(project, name); err != nil {
			return err
		}
	}
	return s.Launch(project, name, false, false, w)
}

// Launch spins a pod that assumes an existing agent's identity. The agent's workspace
// worktree is created on demand; the pod runs interactive Claude in a tmux session
// named after the agent (or a bare shell when shell is true — used for deterministic
// demos and debugging).
func (s *Service) Launch(project, name string, shell, debug bool, progress io.Writer) (err error) {
	ps := s.store.For(project)
	root := s.deps.ProjectRoot(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q — run 'sindri new %s' first", name, name)
	}
	// Validate the project config up front — a bad .sindri/config.yaml fails the launch
	// loudly rather than silently reverting to defaults mid-build.
	cfg, err := s.deps.ProjectConfig(project)
	if err != nil {
		return err
	}
	// Tee build/start progress three ways: the launch buffer (TUI live-screen), the hub
	// log (stderr), and progress — the caller's stream, so `agent start` shows the image
	// build live instead of a frozen prompt (long ops must be visible).
	buf := s.NewLaunchBuf(project, name)
	w := io.MultiWriter(os.Stderr, buf, progress)
	// Pre-flight: podman must be installed and reachable. Fail fast with an actionable
	// message (before touching status or staging an image build). On macOS/Windows this
	// also auto-starts a stopped podman VM, teeing that progress into the launch buffer.
	if err := container.Check(w); err != nil {
		return err
	}
	// Status → launching immediately (cleared by AgentStatus once the pod is up); on any
	// failure below, clear it so it doesn't stick at "launching".
	s.setLifecycle(project, name, "launching")
	_ = ps.Log(name, "launch", "requested")
	s.deps.Notify()
	defer func() {
		if err != nil {
			s.setLifecycle(project, name, "")
			s.deps.Notify()
		}
	}()
	imageRef, err := container.EnsureImage(root, config.Abs(root, cfg.Containerfile), w)
	if err != nil {
		return err
	}
	cName := s.deps.ContainerName(project, name)
	fmt.Fprintf(w, "Image ready. Starting container %s…\n", cName)
	wt := filepath.Join(root, a.Workspace)
	hasCommits, err := git.HasCommits(root)
	if err != nil {
		return err
	}
	if !hasCommits {
		return fmt.Errorf("repo has no commits yet")
	}
	if a.Role == "coauthor" {
		// A coauthor's /workspace IS the user's checkout (wt == repo root) — no isolated
		// worktree to add. Rest in "collab" so the dashboard shows it's standing with the
		// user, not idle.
		if st, _ := ps.GetState(name); st.Phase == "" || st.Phase == "idle" {
			_ = ps.SetState(store.AgentState{Agent: name, Phase: "collab"})
		}
	} else if err := git.WorktreeAdd(root, wt, "HEAD"); err != nil {
		return err
	}
	if a.Role == "planner" {
		// Put the planner on its standing branch so it can draft openspec and ship it via
		// `openspec submit` without ever grabbing a backlog task.
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
	// Serve the agent's own socket BEFORE the pod launches — the pod bind-mounts it, and
	// the socket IS the agent's identity (D2); needs the persistent hub.
	if err := s.agentCh.ServeAgent(project, name); err != nil {
		return err
	}
	workerBin, err := Binary()
	if err != nil {
		return err
	}
	_ = container.Rm(cName) // clear any stale container with this name

	env := map[string]string{"SINDRI_AGENT": name, "COLORTERM": "truecolor"}
	// macOS: the pod can't connect to the bind-mounted unix socket across the VM
	// boundary, so point the worker at the loopback TCP channel with its token. On Linux
	// these are unset and the worker uses /run/sindri/sock (below).
	if runtime.GOOS == "darwin" {
		if s.agentCh.Port() == 0 {
			return fmt.Errorf("agent TCP channel not started — launch needs a persistent hub")
		}
		token, terr := s.Token(project, name)
		if terr != nil {
			return terr
		}
		env["SINDRI_HUB_ADDR"] = fmt.Sprintf("%s:%d", s.agentCh.DialHost(), s.agentCh.Port())
		env["SINDRI_TOKEN"] = token
	}
	mounts := []container.Mount{
		{Host: wt, Container: "/workspace", Mode: "rw"},
		// The agent's own socket — its sole channel to the hub, its identity. Mount the
		// socket DIRECTORY (not the file) so the agent survives a hub restart, which
		// recreates the socket file with a new inode.
		{Host: agentchan.SocketDir(project, name), Container: "/run/sindri", Mode: "rw"},
		// The thin browser binary (image symlinks it to /usr/local/bin/sindri).
		{Host: workerBin, Container: "/opt/sindri/sindri-worker", Mode: "ro"},
	}
	// Mount a cross-built linux brokkr into every pod so the SAME `brokkr` commands work
	// inside the agent regardless of host OS. Runtime mount, so a restart picks it up.
	if bk, berr := BrokkrLinuxBinary(); berr == nil {
		mounts = append(mounts, container.Mount{Host: bk, Container: "/usr/local/bin/brokkr", Mode: "ro"})
	}
	if a.Role == "planner" {
		// A planner sees the whole repo read-only and may only write openspec — so it
		// plans (specs + tasks) without touching code. /workspace is remounted ro and
		// openspec/ overlaid rw on top.
		osDir := filepath.Join(wt, "openspec")
		_ = os.MkdirAll(osDir, 0o755) // ensure the overlay target exists
		mounts[0] = container.Mount{Host: wt, Container: "/workspace", Mode: "ro"}
		mounts = append(mounts, container.Mount{Host: osDir, Container: "/workspace/openspec", Mode: "rw"})
	}
	if shell {
		env["SINDRI_SHELL"] = "1" // entrypoint runs bash instead of Claude
	} else {
		// Compose the agent's system prompt (workflow logic: identity + how-to-work, with
		// the project architecture injected), then hand it to the coding-agent backend to
		// provision its home (credentials, config, prompt) — we own only WHERE it lives.
		archPath := s.deps.ArchitectureDoc(project)
		archContent, _ := os.ReadFile(filepath.Join(root, archPath))
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
		// Mount the user's Claude skills so the agent works with the same skills the user
		// has — read-only and live (host edits show up without a relaunch).
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
		Memory:     MemoryOrDefault(a.Memory),
	}
	if err := container.Run(opts); err != nil {
		return err
	}
	if err := ps.Log(name, "launch", "started container="+cName); err != nil {
		return err
	}
	// Stay until we OBSERVE the agent up (container running AND tmux session answers) —
	// no optimistic "launched" while it's still coming up. On timeout report the failure
	// (deferred cleanup clears the launching intent → board shows "down").
	fmt.Fprintf(w, "Waiting for %s to come up…\n", name)
	deadline := time.Now().Add(launchReadyTimeout)
	shown := 0
	for !s.AgentAlive(project, name) {
		if full := container.Logs(cName, 1000); len(full) > shown { // follow the container's output during the wait
			fmt.Fprint(w, full[shown:])
			shown = len(full)
		}
		if debug { // --debug: surface what the hub's liveness probe actually observes
			fmt.Fprintf(w, "  [debug] %s\n", container.Diagnose(context.Background(), cName))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s launched but didn't come up within %s: %s (check `sindri agent pane %s`)",
				name, launchReadyTimeout, s.LaunchDiagnostic(project, name), name)
		}
		time.Sleep(time.Second)
	}
	s.setLifecycle(project, name, "") // observed up — clear the launching intent now
	fmt.Fprintf(w, "Agent %s is up.\n", name)
	go s.deps.Rehydrate(project, name) // nudge it to resume once the session is live (D13)
	s.deps.Notify()
	return nil
}
