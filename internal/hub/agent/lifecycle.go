// package: hub/agent / lifecycle
// type:    logic (the agent pod lifecycle)
// job:     the mechanics of managing an agent's pod — register (New), Launch a pod
// that assumes its identity, Stop (keep identity), Delete (full teardown),
// Rebuild (fresh image + relaunch), plus the transient launching/stopping
// intent the board reconciles. Triggers come from outside; this does the work.
// limits:  git/container/tmux go through the adapters; the system prompt + branch
// names come from workflow, the coding agent's home from adapter/agent, the
// socket from agentchan. What to inject on rehydrate is the hub's (via Deps).
package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agentchan"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// agentRemoveTimeout bounds tearing a pod down on delete/stop. Wider than probeTimeout: `rm -f`
// stops before it removes, so it is the slowest verb here (-> workflow.runRemoveTimeout).
const agentRemoveTimeout = 30 * time.Second

// lcKey keys the transient lifecycle-intent map by (project, name).
type lcKey struct{ project, name string }

// lifecycleIntent is a transient launch/stop intent plus when it was set (-> LaunchIntent).
type lifecycleIntent struct {
	state string
	since time.Time
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// launchReadyTimeout bounds the wait for a new agent's session. Generous: a cold pod boot
// plus the entrypoint starting tmux takes a while.
const launchReadyTimeout = 45 * time.Second

// setLifecycle records a transient launch/stop intent, cleared by AgentStatus once reality
// catches up. "" clears it.
func (s *Service) setLifecycle(project, name, state string) {
	s.lcMu.Lock()
	defer s.lcMu.Unlock()
	key := lcKey{project, name}
	if state == "" {
		delete(s.lifecycle, key)
	} else {
		s.lifecycle[key] = lifecycleIntent{state: state, since: time.Now()}
	}
}

// clearLaunching retracts a launch intent this call itself set, leaving the sweep's verdict
// (-> FailLaunch) alone: that names a reason, where a bare clear reports "down" — nobody asked.
func (s *Service) clearLaunching(project, name string) {
	s.lcMu.Lock()
	defer s.lcMu.Unlock()
	key := lcKey{project, name}
	if s.lifecycle[key].state == "launching" {
		delete(s.lifecycle, key)
	}
}

// AgentStatus reconciles intent with observed runtime into one status word, clearing the intent
// once fulfilled — the single source of truth for "what is this agent doing". observed is whether
// the runtime has been LOOKED AT at all: running=false alone must never produce "down" or retire an
// intent, since an agent the watchdog has not reached yet supports no claim. stopped is the durable
// flag a human's StopAgent set, checked last of all so it never outranks a truer explanation.
func (s *Service) AgentStatus(project, name string, running, observed bool, phase string, stopped bool) string {
	s.lcMu.Lock()
	defer s.lcMu.Unlock()
	key := lcKey{project, name}
	intent := s.lifecycle[key].state
	switch {
	case intent == "stopping":
		if running || !observed {
			return "stopping" // stop requested; the pod is still up, or nothing has looked yet
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
	case intent == api.StatusLaunchFailed:
		return api.StatusLaunchFailed // the watchdog gave up waiting; see FailLaunch
	case !observed:
		return "unknown" // registered since the last sweep; the next one answers
	case stopped:
		return "stopped" // torn down on purpose, resumable — not the same claim as "down"
	default:
		return "down"
	}
}

// LaunchIntent reports a launch in flight and when it was requested, for the watchdog's own bound
// on how long one may run (-> FailLaunch).
func (s *Service) LaunchIntent(project, name string) (since time.Time, ok bool) {
	s.lcMu.Lock()
	defer s.lcMu.Unlock()
	li := s.lifecycle[lcKey{project, name}]
	return li.since, li.state == "launching"
}

// FailLaunch ends a launch that will never complete: a distinct status rather than a silent fall to
// "down", the reason logged beside "launch: requested", then the container removed under ctx — the
// runtime that hung the launch may hang that too. A no-op unless the intent is still "launching".
func (s *Service) FailLaunch(ctx context.Context, project, name, reason string) {
	s.lcMu.Lock()
	key := lcKey{project, name}
	if s.lifecycle[key].state != "launching" {
		s.lcMu.Unlock()
		return
	}
	s.lifecycle[key] = lifecycleIntent{state: api.StatusLaunchFailed, since: time.Now()}
	s.lcMu.Unlock()
	_ = s.store.For(project).Log(name, "launch", "failed: "+reason)
	s.deps.Notify()
	if err := container.RmContext(ctx, s.deps.ContainerName(project, name)); err != nil {
		_ = s.store.For(project).Log(name, "launch", "container not released: "+err.Error())
	}
}

// whatARoleHolds names what a non-reviewer role carries across its own unit of work — the reason
// none of them can live in a project with no repo to hold it in.
func whatARoleHolds(role string) string {
	switch role {
	case "worker":
		return "a branch"
	case "planner":
		return "a standing conversation"
	case "coauthor":
		return "the user's own seat"
	}
	return "its work"
}

// NewAgent registers an agent identity in a project (no pod) — identity precedes runtime
// (D13). An empty name gets an unused Norse dwarf name. Returns the final name.
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
	// "user" names the human's own mailbox, so an agent called that would share one with the person it
	// reports to, and no row would tell them apart.
	if name == api.SenderUser {
		return "", fmt.Errorf("%q is reserved: it names the user as a mail recipient, so no agent may take it", name)
	}
	if role != "worker" && role != "reviewer" && role != "planner" && role != "coauthor" {
		return "", fmt.Errorf("invalid role %q (worker|reviewer|planner|coauthor)", role)
	}
	if project == api.GlobalProject && role != "reviewer" {
		return "", fmt.Errorf("a %s holds %s across its work — %s has no repo to hold it in, so only a reviewer can be created there",
			role, whatARoleHolds(role), api.GlobalProject)
	}
	if !ValidMemory(memory) {
		return "", fmt.Errorf("invalid memory %q (e.g. 2g, 512m)", memory)
	}
	// Unique across ALL repos, so the unified board never shows two agents with one name.
	agents, err := s.store.AllAgents()
	if err != nil {
		return "", err
	}
	for _, a := range agents {
		if a.Name == name {
			return "", fmt.Errorf("agent %q already exists (in %s) — names are unique across all repos", name, a.Project)
		}
	}
	// A coauthor shares the user's real checkout, not a worktree — the SAME material.
	workspace := filepath.Join(workflow.AgentTrees, name)
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

// DeleteAgent removes an agent entirely — pod, socket, worktree, identity, log. Teardown is
// best-effort (a missing pod or worktree is fine); the identity always goes.
func (s *Service) DeleteAgent(ctx context.Context, project, name string) error {
	ps := s.store.For(project)
	root := s.deps.ProjectRoot(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	// Release the task so it isn't stranded in_progress with no owner. Only a task sindri owns has
	// a status to release; a gh-/os- item's is inferred from agent_state.
	if st, _ := ps.GetState(name); ps.OwnsTask(st.Task) {
		if err := ps.SetOwnedStatus(st.Task, "open"); err != nil {
			log.Printf("hub: reopen %s on delete of %s: %v", st.Task, name, err)
		}
		_ = s.deps.RefreshTask(project, st.Task)
	}
	rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), agentRemoveTimeout)
	_ = container.RmContext(rmCtx, s.deps.ContainerName(project, name))
	rmCancel()
	s.agentCh.CloseAgent(project, name)
	switch {
	// GlobalProject's workspace is a plain directory (prepareWorkspace's own doing), not a
	// worktree — git.WorktreeRemove against it fails and leaves the tree behind forever.
	case project == workflow.GlobalProject:
		_ = os.RemoveAll(filepath.Join(root, a.Workspace))
	// A coauthor's workspace IS the repo root — never `git worktree remove` that. Its scratch tree
	// goes: left behind, the next agent of that name would inherit it.
	case a.Workspace != ".":
		_ = git.WorktreeRemove(root, filepath.Join(root, a.Workspace))
	default:
		_ = git.WorktreeRemove(root, filepath.Join(root, workflow.ScratchWorktree(name)))
	}
	if err := ps.DeleteAgent(name); err != nil {
		return err
	}
	s.deps.Notify()
	return nil
}

// StopAgent tears down the pod but keeps identity, worktree, socket and log, so a relaunch
// resumes where it left off.
func (s *Service) StopAgent(ctx context.Context, project, name string) error {
	return s.stopAgent(ctx, project, name, "pod removed")
}

// stopAgent is StopAgent with the log line's reason as the caller's — human-requested by default,
// but the idle sweep states what it acted on instead (-> FireIdleStops), since this is the hub
// acting on the fleet unasked and a user who finds an agent stopped must be able to see why.
func (s *Service) stopAgent(ctx context.Context, project, name, reason string) error {
	ps := s.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if !container.RunningContext(ctx, s.deps.ContainerName(project, name)) {
		return fmt.Errorf("agent %q is not running", name)
	}
	s.setLifecycle(project, name, "stopping") // status → stopping (pod up); → down once gone
	s.deps.Notify()
	rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), agentRemoveTimeout)
	err = container.RmContext(rmCtx, s.deps.ContainerName(project, name))
	rmCancel()
	if err != nil {
		s.setLifecycle(project, name, "")
		s.deps.Notify()
		return err
	}
	// Durable, so "stopped" (resumable) reads distinct from "down" (crashed) even across a hub
	// restart that drops the in-memory intent above.
	a.Stopped = true
	_ = ps.PutAgent(a)
	_ = ps.Log(name, "stop", reason)
	s.deps.Notify()
	return nil
}

// RebuildAgent rebuilds the image (re-pull base) then relaunches, streaming progress to w. A
// bad config fails before any build; a running agent stops first so it comes up on the new one.
func (s *Service) RebuildAgent(ctx context.Context, project, name string, w io.Writer) error {
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
	if container.RunningContext(ctx, s.deps.ContainerName(project, name)) {
		fmt.Fprintf(w, "Image rebuilt — restarting %s to run it (the session resumes)…\n", name)
	}
	return s.RestartAgent(ctx, project, name, w)
}

// RestartAgent replaces an agent's pod with a fresh one on the same identity — the worktree,
// socket and log survive, so the session resumes and a down agent is simply started. It is also
// the remedy for a signed-out one: the new process reads the credentials the hub keeps staged.
func (s *Service) RestartAgent(ctx context.Context, project, name string, w io.Writer) error {
	if container.RunningContext(ctx, s.deps.ContainerName(project, name)) {
		if err := s.StopAgent(ctx, project, name); err != nil {
			return err
		}
	}
	return s.Launch(ctx, project, name, false, false, 0, 0, w)
}

// plannerWritable is the one directory a planner may write, overlaid on a read-only workspace.
const plannerWritable = "openspec"

// workspaceMounts is everything a pod can reach in the repo: its workspace, plus what the two
// exceptions to isolation add. A coauthor's is the user's own checkout, which HOLDS every other
// agent's worktree — hidden behind an empty directory, since the hub commits from those trees — so
// it gets a scratch tree of its own to check work out into instead.
func workspaceMounts(role, wt, hidden, scratch string) []container.Mount {
	ws := container.Mount{Host: wt, Container: "/workspace", Mode: "rw"}
	switch role {
	case "planner":
		ws.Mode = "ro"
		return []container.Mount{ws,
			{Host: filepath.Join(wt, plannerWritable), Container: "/workspace/" + plannerWritable, Mode: "rw"}}
	case "coauthor":
		return []container.Mount{ws,
			{Host: hidden, Container: "/workspace/" + workflow.AgentTrees, Mode: "ro"},
			{Host: scratch, Container: workflow.ScratchMount, Mode: "rw"}}
	}
	return []container.Mount{ws}
}

// previewSizeEnv sizes a session's tmux pane at creation (-> sindri-agent.sh), or nothing when
// either dimension is unset — the CLI's case, left at tmux's own default until attached.
func previewSizeEnv(cols, lines int) map[string]string {
	if cols <= 0 || lines <= 0 {
		return nil
	}
	return map[string]string{"SINDRI_COLS": strconv.Itoa(cols), "SINDRI_LINES": strconv.Itoa(lines)}
}

// modelEnv is the model to launch on (-> sindri-agent.sh, both the --model flag and the status
// line), or nothing when none is chosen — the account default, same as always.
func modelEnv(model string) map[string]string {
	if model == "" {
		return nil
	}
	return map[string]string{"SINDRI_MODEL": model}
}

// prepareWorkspace lays down what /workspace will bind-mount: nothing to check out for a
// GlobalProject reviewer (-> workflow.assignReview fills it later), else a git worktree shaped
// by role.
func (s *Service) prepareWorkspace(ps *store.ProjectStore, project, name, root, wt string, a store.Agent) error {
	if project == workflow.GlobalProject {
		return os.MkdirAll(wt, 0o755)
	}
	hasCommits, err := git.HasCommits(root)
	if err != nil {
		return err
	}
	if !hasCommits {
		return fmt.Errorf("repo has no commits yet")
	}
	if a.Role == "coauthor" {
		// A coauthor's /workspace IS the user's checkout (wt == repo root), so there is no isolated
		// worktree to add — only its scratch tree (-> CmdScratch), kept as it was on a relaunch.
		if err := git.WorktreeAdd(root, filepath.Join(root, workflow.ScratchWorktree(name)), "HEAD"); err != nil {
			return err
		}
		// Rest in "collab" so the dashboard shows it's standing with the user, not idle.
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
	return nil
}

// Launch spins a pod that assumes an existing agent's identity, running Claude in a tmux session
// named after it (or a bare shell); cols/lines size it to a caller's preview pane.
func (s *Service) Launch(ctx context.Context, project, name string, shell, debug bool, cols, lines int, progress io.Writer) (err error) {
	ps := s.store.For(project)
	root := s.deps.ProjectRoot(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q — run 'sindri new %s' first", name, name)
	}
	if a.Stopped { // asked to run again — no longer the human's deliberate down
		a.Stopped = false
		_ = ps.PutAgent(a)
	}
	// Status → launching before any preflight, not after: container.Check below can start a
	// stopped podman VM on macOS, and that wait must not read as "down" for having asked nothing yet.
	s.setLifecycle(project, name, "launching")
	_ = ps.Log(name, "launch", "requested")
	s.deps.Notify()
	defer func() {
		if err != nil {
			s.clearLaunching(project, name)
			s.deps.Notify()
		}
	}()
	// Validate the project config up front — a bad .sindri/config.yaml fails the launch
	// loudly rather than silently reverting to defaults mid-build.
	cfg, err := s.deps.ProjectConfig(project)
	if err != nil {
		return err
	}
	// Tee progress three ways — launch buffer (TUI), hub log, caller's stream — so an image
	// build shows live rather than as a frozen prompt.
	buf := s.NewLaunchBuf(project, name)
	w := io.MultiWriter(os.Stderr, buf, progress)
	// Pre-flight podman before touching status or staging a build; on macOS this also starts
	// a stopped VM.
	if err := container.Check(w); err != nil {
		return err
	}
	imageRef, err := container.EnsureImage(root, config.Abs(root, cfg.Containerfile), w)
	if err != nil {
		return err
	}
	cName := s.deps.ContainerName(project, name)
	fmt.Fprintf(w, "Image ready. Starting container %s…\n", cName)
	wt := filepath.Join(root, a.Workspace)
	if err := s.prepareWorkspace(ps, project, name, root, wt, a); err != nil {
		return err
	}
	// Serve the agent's own socket BEFORE the pod launches — the pod bind-mounts it, and
	// the socket IS the agent's identity (D2); needs the persistent hub.
	if err := s.agentCh.ServeAgent(project, name); err != nil {
		return err
	}
	// Fill pod-bin before the pod mounts it. Logged, not fatal — a host may have no brokkr.
	if updated, serr := SyncPodBin(); serr != nil {
		fmt.Fprintf(os.Stderr, "hub: pod-bin sync: %v\n", serr)
	} else if len(updated) > 0 {
		fmt.Fprintf(os.Stderr, "hub: pod-bin refreshed: %s\n", strings.Join(updated, ", "))
	}
	if _, err := Binary(); err != nil { // the worker is required; fail before launching
		return err
	}
	_ = container.RmContext(ctx, cName) // clear any stale container with this name

	env := map[string]string{"SINDRI_AGENT": name, "COLORTERM": "truecolor"}
	for k, v := range previewSizeEnv(cols, lines) {
		env[k] = v
	}
	for k, v := range modelEnv(a.Model) {
		env[k] = v
	}
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
	// An overlay target must exist on the host before podman binds over it, and each belongs to one
	// role (-> workspaceMounts).
	switch a.Role {
	case "planner":
		_ = os.MkdirAll(filepath.Join(wt, plannerWritable), 0o755)
	case "coauthor":
		if err := os.MkdirAll(paths.HiddenDir(), 0o755); err != nil {
			return err
		}
	}
	mounts := append(workspaceMounts(a.Role, wt, paths.HiddenDir(), filepath.Join(root, workflow.ScratchWorktree(name))),
		// The agent's own socket — its sole channel to the hub, its identity. Mount the
		// socket DIRECTORY (not the file) so the agent survives a hub restart, which
		// recreates the socket file with a new inode.
		container.Mount{Host: agentchan.SocketDir(project, name), Container: "/run/sindri", Mode: "rw"},
		// The host-built tools (brokkr, the browser) as ONE DIRECTORY, for the same reason
		// as the socket above: a per-file bind pins the inode, so a reinstalled binary
		// never reached the pod. The image symlinks /usr/local/bin/{brokkr,sindri} in here.
		container.Mount{Host: paths.PodBinDir(), Container: paths.PodBinMount, Mode: "ro"})
	if shell {
		env["SINDRI_SHELL"] = "1" // entrypoint runs bash instead of Claude
	} else {
		// Compose the agent's system prompt (workflow logic: identity + how-to-work, with
		// the project architecture injected), then hand it to the coding-agent backend to
		// provision its home (credentials, config, prompt) — we own only WHERE it lives.
		archPath := s.deps.ArchitectureDoc(project)
		archContent, _ := os.ReadFile(filepath.Join(root, archPath))
		sysPrompt := workflow.SystemPrompt(name, a.Role, string(archContent), archPath)
		homeDir := paths.AgentHomeDir(project, name)
		home, err := agentport.PrepareHome(agentport.HomeSpec{Dir: homeDir, SystemPrompt: sysPrompt, Out: w, Workspace: wt})
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
	for !s.AgentAlive(ctx, project, name) {
		if full := container.Logs(cName, 1000); len(full) > shown { // follow the container's output during the wait
			fmt.Fprint(w, full[shown:])
			shown = len(full)
		}
		if debug { // --debug: surface what the hub's liveness probe actually observes
			fmt.Fprintf(w, "  [debug] %s\n", container.Diagnose(ctx, cName))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s launched but didn't come up within %s: %s (check `sindri agent pane %s`)",
				name, launchReadyTimeout, s.LaunchDiagnostic(ctx, project, name), name)
		}
		time.Sleep(time.Second)
	}
	s.setLifecycle(project, name, "") // observed up — clear the launching intent now
	fmt.Fprintf(w, "Agent %s is up.\n", name)
	go s.deps.Rehydrate(project, name) // nudge it to resume once the session is live (D13)
	s.deps.Notify()
	return nil
}
