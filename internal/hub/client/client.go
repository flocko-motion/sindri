// package: hub/client / client
// type:    logic (the hub's wire client)
// job:     the thin client every host-side caller (CLI, TUI) uses to talk to a
//          running hub over its unix socket. Mirrors the hub's operation set so
//          it is interchangeable with an in-process hub. Lives under hub/ as the
//          client side of the hub's own API.
// limits:  no domain logic; just marshals calls to the hub's HTTP API.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// HTTP talks to a hub over its repo unix socket.
type HTTP struct {
	hc   *http.Client
	base string
}

// DialSocket returns a client that talks to the hub over a specific unix socket —
// used by the in-pod worker on Linux, whose socket IS its identity (no header).
func DialSocket(socketPath string) *HTTP {
	return &HTTP{base: "http://unix", hc: &http.Client{Transport: unixTransport(socketPath)}}
}

// Dial returns a host client for the single global hub, tagging every request with
// the repo it concerns (X-Sindri-Project = the repo root) so the hub scopes to that
// project. The ~repo context rides at the transport layer, so callers don't thread
// it through each method.
func Dial(root string) *HTTP {
	return &HTTP{base: "http://unix", hc: &http.Client{Transport: &headerRT{
		key: "X-Sindri-Project", val: root, rt: unixTransport(hub.SocketPath())}}}
}

// DialTCP returns a client that talks to the hub over TCP, presenting token on
// every request as its identity — the in-pod worker on macOS, where a bind-mounted
// unix socket can't cross the podman VM boundary.
func DialTCP(addr, token string) *HTTP {
	return &HTTP{base: "http://" + addr, hc: &http.Client{Transport: &headerRT{
		key: "X-Sindri-Token", val: token, rt: http.DefaultTransport}}}
}

// unixTransport dials the given unix socket for every request.
func unixTransport(socketPath string) *http.Transport {
	return &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	}}
}

// headerRT adds a fixed header — the caller's project (host) or token (agent) — to
// every request; that header is the caller's identity/context on the wire.
type headerRT struct {
	key, val string
	rt       http.RoundTripper
}

func (t *headerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context()) // don't mutate the caller's request (RoundTripper contract)
	r.Header.Set(t.key, t.val)
	return t.rt.RoundTrip(r)
}

// Close is a no-op (kept so HTTP satisfies the same interface as *hub.Hub).
func (c *HTTP) Close() error { return nil }

// State fetches the whole board (agents, tasks, PRs, orphans).
func (c *HTTP) State() (hub.BoardState, error) {
	var out hub.BoardState
	return out, c.get("/state", &out)
}

// Stats returns the wired engine plus a memory snapshot for every running agent
// across all repos (the data behind `agent stats`).
func (c *HTTP) Stats() (hub.StatsReport, error) {
	var out hub.StatsReport
	return out, c.get("/stats", &out)
}

// Instance returns the engine + container instance detail for one agent (engine,
// container name, state, image, cpus, memory limit, platform, host pid) — the
// identity behind `agent info`.
func (c *HTTP) Instance(name string) (string, error) {
	var ok struct {
		Out string `json:"ok"`
	}
	err := c.get("/agent/pod?agent="+url.QueryEscape(name), &ok)
	return ok.Out, err
}

// Watch subscribes to board-state changes over SSE. It returns a channel that
// yields the current state on connect and a fresh snapshot on every change; the
// channel closes when ctx is cancelled or the hub goes away.
func (c *HTTP) Watch(ctx context.Context) (<-chan hub.BoardState, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/events", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	out := make(chan hub.BoardState)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var st hub.BoardState
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &st) != nil {
				continue
			}
			select {
			case out <- st:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NewAgent registers an agent identity (empty name ⇒ hub auto-names it). memory is
// an optional RAM limit (e.g. "4g"; "" = hub default). Returns the final name.
func (c *HTTP) NewAgent(name, role, memory string) (string, error) {
	var ok struct {
		Name string `json:"ok"`
	}
	err := c.postResult("/agents", hub.AgentReq{Name: name, Role: role, Memory: memory}, &ok)
	return ok.Name, err
}

// SetMemory sets an agent's RAM limit (e.g. "4g"; "" resets to the hub default).
// Takes effect on the agent's next start/restart.
func (c *HTTP) SetMemory(name, memory string) error {
	return c.post("/agent/memory", hub.NameReq{Name: name, Memory: memory})
}

// DeleteAgent removes an agent (pod, socket, worktree, identity).
func (c *HTTP) DeleteAgent(name string) error {
	return c.post("/agent/delete", hub.NameReq{Name: name})
}

// StopAgent tears down the agent's pod but keeps its identity.
func (c *HTTP) StopAgent(name string) error {
	return c.post("/agent/stop", hub.NameReq{Name: name})
}

// RebaseAgent rebases the agent's worktree onto the current base (reference) branch.
func (c *HTTP) RebaseAgent(name string) error {
	return c.post("/agent/rebase", hub.NameReq{Name: name})
}

// AgentPane returns the last `lines` rows of the agent's tmux pane (plain text).
func (c *HTTP) AgentPane(name string, lines int) (string, error) {
	var ok struct {
		Out string `json:"ok"`
	}
	err := c.get(fmt.Sprintf("/agent/pane?agent=%s&lines=%d", url.QueryEscape(name), lines), &ok)
	return ok.Out, err
}

// Diagnose asks the hub what its liveness probes observe for an agent (running
// check + session check, with a wedged exec surfaced as a timeout) — the detail
// behind `agent info --debug` that explains a "down" contradicting a live pod.
func (c *HTTP) Diagnose(name string) (string, error) {
	var ok struct {
		Out string `json:"ok"`
	}
	err := c.get("/agent/diagnose?agent="+url.QueryEscape(name), &ok)
	return ok.Out, err
}

// Clients returns the humans attached to an agent's tmux session (dial-ins).
func (c *HTTP) Clients(name string) ([]hub.ClientView, error) {
	var out []hub.ClientView
	return out, c.get("/agent/clients?agent="+url.QueryEscape(name), &out)
}

// PodInfo returns a short summary of an agent's podman container (plain text).
func (c *HTTP) PodInfo(name string) (string, error) {
	var ok struct {
		Out string `json:"ok"`
	}
	err := c.get("/agent/pod?agent="+url.QueryEscape(name), &ok)
	return ok.Out, err
}

// Launch spins a pod for an existing agent (shell=true runs a bare shell instead
// of Claude), streaming the hub's build/start progress to out so a long image
// build isn't a frozen prompt. debug=true streams the hub's liveness-probe detail
// during the wait. The failure, if any, rides back in a trailer.
func (c *HTTP) Launch(name string, shell, debug bool, out io.Writer) error {
	body, err := json.Marshal(hub.NameReq{Name: name, Shell: shell, Debug: debug})
	if err != nil {
		return err
	}
	resp, err := c.hc.Post(c.base+"/launch", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(out, resp.Body)
	if e := resp.Trailer.Get("X-Sindri-Error"); e != "" {
		return fmt.Errorf("%s", e)
	}
	return nil
}

// RebuildImage force-rebuilds the agent's image (re-pulling the base) and relaunches
// it; the build/restart progress streams to out, the failure (if any) via a trailer.
func (c *HTTP) RebuildImage(name string, out io.Writer) error {
	body, err := json.Marshal(hub.NameReq{Name: name})
	if err != nil {
		return err
	}
	resp, err := c.hc.Post(c.base+"/agent/rebuild", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(out, resp.Body)
	if e := resp.Trailer.Get("X-Sindri-Error"); e != "" {
		return fmt.Errorf("%s", e)
	}
	return nil
}

// Tell delivers a provenance-stamped message into an agent's session.
func (c *HTTP) Tell(name, msg, source string) error {
	return c.post("/tell", hub.TellReq{Name: name, Msg: msg, Source: source})
}

// ChatAdd adds an agent to the user's chatroom (the hub greets it).
func (c *HTTP) ChatAdd(name string) error {
	return c.post("/chat/add", hub.NameReq{Name: name})
}

// ChatRemove takes an agent out of the chatroom.
func (c *HTTP) ChatRemove(name string) error {
	return c.post("/chat/remove", hub.NameReq{Name: name})
}

// ChatSay posts a message to the chatroom as the user (the discussion leader).
func (c *HTTP) ChatSay(msg string) error {
	return c.post("/chat/say", hub.ChatSayReq{Msg: msg})
}

// ChatHeartbeat signals the user is present in the chatroom (sent periodically by
// `chat join` and the TUI chat tab). Presence keeps the room unlocked for agents.
func (c *HTTP) ChatHeartbeat() error {
	return c.post("/chat/heartbeat", struct{}{})
}

// Chat returns the current chatroom snapshot (members + recent transcript).
func (c *HTTP) Chat() (hub.ChatView, error) {
	var v hub.ChatView
	return v, c.get("/chat", &v)
}

// ChatWatch subscribes to the chatroom over SSE: it yields the snapshot on connect
// and a fresh one on every change, closing when ctx is cancelled or the hub goes
// away. This is the user's live leg of the star topology (the join CLI, TUI tab).
func (c *HTTP) ChatWatch(ctx context.Context) (<-chan hub.ChatView, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/chat/stream", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	out := make(chan hub.ChatView)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var v hub.ChatView
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v) != nil {
				continue
			}
			select {
			case out <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// Commands fetches the caller's currently-available command surface (the browser
// menu). Identity is the socket, so no name is sent.
func (c *HTTP) Commands() ([]hub.CmdInfo, error) {
	var out []hub.CmdInfo
	return out, c.get("/commands", &out)
}

// Directive returns the hub's single next-action instruction for this agent.
func (c *HTTP) Directive() (string, error) {
	var ok struct {
		Directive string `json:"ok"`
	}
	return ok.Directive, c.get("/directive", &ok)
}

// Exec runs a verb on the hub, streaming output to out, and returns the
// command's exit code (carried back in the X-Sindri-Exit trailer).
func (c *HTTP) Exec(args []string, out io.Writer) (int, error) {
	buf, err := json.Marshal(hub.ExecReq{Args: args})
	if err != nil {
		return 1, err
	}
	resp, err := c.hc.Post(c.base+"/exec", "application/json", bytes.NewReader(buf))
	if err != nil {
		return 1, err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return 1, err
	}
	if v := resp.Trailer.Get("X-Sindri-Exit"); v != "" {
		if code, err := strconv.Atoi(v); err == nil {
			return code, nil
		}
	}
	return 0, nil
}

// Merge merges an approved PR (host/human-only gate). Returns the merged PR.
func (c *HTTP) Merge(id string) (store.PR, error) {
	var pr store.PR
	buf, err := json.Marshal(hub.NameReq{Name: id})
	if err != nil {
		return pr, err
	}
	resp, err := c.hc.Post(c.base+"/merge", "application/json", bytes.NewReader(buf))
	if err != nil {
		return pr, err
	}
	defer resp.Body.Close()
	return pr, readResult(resp, &pr)
}

// MilestonePR opens a milestone PR for the container an agent is collaborating
// on — blocking that agent until the human merges.
func (c *HTTP) MilestonePR(agent string) (store.PR, error) {
	var pr store.PR
	buf, err := json.Marshal(hub.NameReq{Name: agent})
	if err != nil {
		return pr, err
	}
	resp, err := c.hc.Post(c.base+"/milestone", "application/json", bytes.NewReader(buf))
	if err != nil {
		return pr, err
	}
	defer resp.Body.Close()
	return pr, readResult(resp, &pr)
}

// PRs lists all merge-intents.
func (c *HTTP) PRs() ([]store.PR, error) {
	var out []store.PR
	return out, c.get("/prs", &out)
}

// PRInfo returns a PR with its diff.
func (c *HTTP) PRInfo(id string) (hub.PRDetail, error) {
	var d hub.PRDetail
	return d, c.get("/pr?id="+url.QueryEscape(id), &d)
}

// RejectPR rejects a PR with feedback, routed to the owning worker.
func (c *HTTP) RejectPR(id, feedback string) error {
	return c.post("/pr/reject", hub.RejectReq{ID: id, Feedback: feedback})
}

// ApprovePR marks an open PR approved (the human path), so it can be merged
// without a reviewer agent.
func (c *HTTP) ApprovePR(id string) error {
	return c.post("/pr/approve", hub.NameReq{Name: id})
}

// LintPR runs the quality gate against a PR's worktree and returns the output.
func (c *HTTP) LintPR(id string) (string, error) {
	var ok struct {
		Out string `json:"ok"`
	}
	return ok.Out, c.get("/pr/lint?id="+url.QueryEscape(id), &ok)
}

// RequestReview attaches a review requirement to a PR and dispatches it to a
// reviewer agent.
func (c *HTTP) RequestReview(id, requirement string) error {
	return c.post("/pr/review", hub.RejectReq{ID: id, Feedback: requirement})
}

// ReviewPrompt returns the editable default agentic-review instruction.
func (c *HTTP) ReviewPrompt() (string, error) {
	var ok struct {
		Prompt string `json:"ok"`
	}
	return ok.Prompt, c.get("/review-prompt", &ok)
}

// MaterializeReview checks a PR out into the reserved review workspace and
// returns the path.
func (c *HTTP) MaterializeReview(id string) (string, error) {
	var ok struct {
		Path string `json:"ok"`
	}
	return ok.Path, c.get("/pr/materialize?id="+url.QueryEscape(id), &ok)
}

// Tasks lists all cached tasks (refreshed from the source of truth).
func (c *HTTP) Tasks() ([]store.Task, error) {
	var out []store.Task
	return out, c.get("/tasks", &out)
}

// TaskInfo returns one task (refreshed first).
func (c *HTTP) TaskInfo(id string) (store.Task, error) {
	var t store.Task
	return t, c.get("/task?id="+url.QueryEscape(id), &t)
}

// ReconcileTasks repairs stale task statuses against reality (a startup sweep).
func (c *HTTP) ReconcileTasks() error {
	return c.post("/tasks/reconcile", struct{}{})
}

// RefreshTaskComments forces a re-sync of one task's comments from its source
// (the [r]efresh key), bypassing the TTL.
func (c *HTTP) RefreshTaskComments(id string) error {
	return c.post("/task/comments/refresh", hub.NameReq{Name: id})
}

// CreateTask creates a task from a spec and returns its id.
func (c *HTTP) CreateTask(s hub.TaskSpec) (string, error) {
	var ok struct {
		ID string `json:"ok"`
	}
	err := c.postResult("/tasks", specReq("", s), &ok)
	return ok.ID, err
}

// EditTask applies a spec to an existing task.
func (c *HTTP) EditTask(id string, s hub.TaskSpec) error {
	return c.post("/task/edit", specReq(id, s))
}

func specReq(id string, s hub.TaskSpec) hub.TaskReq {
	return hub.TaskReq{ID: id, Title: s.Title, Type: s.Type, Priority: s.Priority, Parent: s.Parent, Description: s.Description, Labels: s.Labels}
}

// SetPriority assigns a task's priority (P-code) — to td or our own db.
func (c *HTTP) SetPriority(id, priority string) error {
	return c.post("/priority", hub.PriorityReq{ID: id, Priority: priority})
}

// ApproveTask clears the approval gate on a planner-proposed task.
func (c *HTTP) ApproveTask(id string) error {
	return c.post("/task/approve", hub.RejectReq{ID: id})
}

// RejectTask rejects a planner-proposed task with a comment.
func (c *HTTP) RejectTask(id, comment string) error {
	return c.post("/task/reject", hub.RejectReq{ID: id, Feedback: comment})
}

// UnassignTask releases a task back to the backlog (refused if a live agent holds it).
func (c *HTTP) UnassignTask(id string) error {
	return c.post("/task/unassign", hub.RejectReq{ID: id})
}

// CloseTask marks a task done from the task list (the "done" close). The hub
// dispatches to the task's backend (td close / openspec archive / issue close).
func (c *HTTP) CloseTask(id string) error {
	return c.post("/task/close", hub.RejectReq{ID: id})
}

// DeleteTask scraps a task from the task list (the "discard" close). The hub
// dispatches to the backend (td delete / openspec change-dir removal / issue delete).
func (c *HTTP) DeleteTask(id string) error {
	return c.post("/task/delete", hub.RejectReq{ID: id})
}

// Refresh asks the hub to re-sync tasks from the source of truth.
func (c *HTTP) Refresh() error { return c.post("/refresh", struct{}{}) }

// Log fetches an agent's recent activity-log entries (the timeline).
func (c *HTTP) Log(name string) ([]store.Event, error) {
	var out []store.Event
	return out, c.get("/log?agent="+url.QueryEscape(name), &out)
}

// Repos lists every registered repo (the registry overview / TUI switcher source).
func (c *HTTP) Repos() ([]hub.RepoSummary, error) {
	var out []hub.RepoSummary
	return out, c.get("/repos", &out)
}

// RepoInfo returns one repo's resolved config + counts; an empty tag defaults to the
// caller's repo (the client's X-Sindri-Project).
func (c *HTTP) RepoInfo(tag string) (hub.RepoDetail, error) {
	var out hub.RepoDetail
	path := "/repo"
	if tag != "" {
		path += "?tag=" + url.QueryEscape(tag)
	}
	return out, c.get(path, &out)
}

// RepoInit registers the caller's repo and scaffolds its .sindri/config.yaml.
func (c *HTTP) RepoInit() (hub.RepoSummary, error) {
	var out hub.RepoSummary
	return out, c.postResult("/repo/init", struct{}{}, &out)
}

// RepoForget drops a repo from the registry by tag (files untouched, agent-guarded).
func (c *HTTP) RepoForget(tag string) error {
	return c.post("/repo/forget", hub.RepoReq{Tag: tag})
}

// SetRepoColor pins a repo's colour choice by tag (0 = hash-derived default).
func (c *HTTP) SetRepoColor(tag string, color int) error {
	return c.post("/repo/color", hub.RepoReq{Tag: tag, Color: color})
}

// RemoveOrphan removes a stray container by name (a running pod with no roster entry).
func (c *HTTP) RemoveOrphan(name string) error {
	return c.post("/orphan/remove", hub.NameReq{Name: name})
}

// WriteRepoConfig persists the caller's repo's .sindri/config.yaml (validated hub-side).
func (c *HTTP) WriteRepoConfig(cfg config.Config) error {
	return c.post("/repo/config", cfg)
}

func (c *HTTP) get(path string, out any) error {
	resp, err := c.hc.Get(c.base + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readResult(resp, out)
}

func (c *HTTP) post(path string, body any) error {
	return c.postResult(path, body, nil)
}

// postResult posts body and decodes a successful JSON response into out.
func (c *HTTP) postResult(path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.hc.Post(c.base+path, "application/json", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readResult(resp, out)
}

// readResult decodes a successful JSON body into out (if non-nil), or turns a
// non-2xx into the hub's reported error.
func readResult(resp *http.Response, out any) error {
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("hub error: %s", resp.Status)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
