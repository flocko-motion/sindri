// package: client / client
// type:    logic (the hub's wire client)
// job:     the thin client every host-side caller (CLI, TUI, cmd/sindri-worker) uses
// to talk to a running hub over its unix socket. Mirrors the hub's operation set so
// it is interchangeable with an in-process hub.
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

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/tools/paths"
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
		key: "X-Sindri-Project", val: root, rt: unixTransport(paths.HubSocket())}}}
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
func (c *HTTP) State() (api.BoardState, error) {
	var out api.BoardState
	return out, c.get("/state", &out)
}

// Stats returns the wired engine plus a memory snapshot for every running agent
// across all repos (the data behind `agent stats`).
func (c *HTTP) Stats() (api.StatsReport, error) {
	var out api.StatsReport
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
func (c *HTTP) Watch(ctx context.Context) (<-chan api.BoardState, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/events", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	out := make(chan api.BoardState)
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
			var st api.BoardState
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
	err := c.postResult("/agents", api.AgentReq{Name: name, Role: role, Memory: memory}, &ok)
	return ok.Name, err
}

// SetMemory sets an agent's RAM limit (e.g. "4g"; "" resets to the hub default).
// Takes effect on the agent's next start/restart.
func (c *HTTP) SetMemory(name, memory string) error {
	return c.post("/agent/memory", api.NameReq{Name: name, Memory: memory})
}

// SetRetired winds an agent down (retired=true): it finishes what it holds and is handed nothing
// new. false puts it back in service.
func (c *HTTP) SetRetired(name string, retired bool) error {
	return c.post("/agent/retire", api.NameReq{Name: name, Retired: retired})
}

// ResumeAgent clears an escalation from the host — the user's half of it. The agent clears its own
// once it has the answer; this is for one that cannot, or should not have escalated at all.
func (c *HTTP) ResumeAgent(name string) error {
	return c.post("/agent/resume", api.NameReq{Name: name})
}

// DeleteAgent removes an agent (pod, socket, worktree, identity).
func (c *HTTP) DeleteAgent(name string) error {
	return c.post("/agent/delete", api.NameReq{Name: name})
}

// StopAgent tears down the agent's pod but keeps its identity.
func (c *HTTP) StopAgent(name string) error {
	return c.post("/agent/stop", api.NameReq{Name: name})
}

// SetClearArmed arms a context clear (armed=true), which fires at the agent's next leaf boundary —
// at once if it is already at one — or takes the arming back (false).
func (c *HTTP) SetClearArmed(name string, armed bool) error {
	return c.post("/agent/clear-context", api.NameReq{Name: name, Armed: armed})
}

// RebaseAgent rebases the agent's worktree onto the current base (reference) branch.
func (c *HTTP) RebaseAgent(name string) error {
	return c.post("/agent/rebase", api.NameReq{Name: name})
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
func (c *HTTP) Clients(name string) ([]api.ClientView, error) {
	var out []api.ClientView
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
// during the wait. cols/lines size the session's tmux preview at creation (0, 0 for
// no preview — the CLI's case). The failure, if any, rides back in a trailer.
func (c *HTTP) Launch(name string, shell, debug bool, cols, lines int, out io.Writer) error {
	body, err := json.Marshal(api.NameReq{Name: name, Shell: shell, Debug: debug, Cols: cols, Lines: lines})
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
	body, err := json.Marshal(api.NameReq{Name: name})
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

// AssignPlan gives a planner one thing to plan, as a phased brief. taskID works up an existing task,
// which becomes the parent of what the planning produces; goal alone plans free text. Refused while
// that planner has a PR open — the answer says to merge or scrap it first.
func (c *HTTP) AssignPlan(name, goal, taskID string) error {
	return c.post("/agent/plan", api.PlanReq{Name: name, Goal: goal, Task: taskID})
}

// Commands fetches the caller's currently-available command surface (the browser
// menu). Identity is the socket, so no name is sent.
func (c *HTTP) Commands() ([]api.CmdInfo, error) {
	var out []api.CmdInfo
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
	buf, err := json.Marshal(api.ExecReq{Args: args})
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
func (c *HTTP) Merge(id string) (api.PR, error) {
	var pr api.PR
	buf, err := json.Marshal(api.NameReq{Name: id})
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
func (c *HTTP) MilestonePR(agent string) (api.PR, error) {
	var pr api.PR
	buf, err := json.Marshal(api.NameReq{Name: agent})
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
func (c *HTTP) PRs() ([]api.PR, error) {
	var out []api.PR
	return out, c.get("/prs", &out)
}

// PRInfo returns a PR with its diff.
func (c *HTTP) PRInfo(id string) (api.PRDetail, error) {
	var d api.PRDetail
	return d, c.get("/pr?id="+url.QueryEscape(id), &d)
}

// NextTask explains what would be handed out next and why nothing else would be. Ask about an
// agent, or about a role as a hypothetical agent of it holding nothing (a reviewer's answer is
// PRs, being the pool it is served from); both together are refused, and both empty is the
// backlog question a worker would be asked.
func (c *HTTP) NextTask(agent, role string) (api.NextExplain, error) {
	var x api.NextExplain
	return x, c.get("/task/next?agent="+url.QueryEscape(agent)+"&role="+url.QueryEscape(role), &x)
}

// RejectPR rejects a PR with feedback, routed to the owning worker.
func (c *HTTP) RejectPR(id, feedback string) error {
	return c.post("/pr/reject", api.RejectReq{ID: id, Feedback: feedback})
}

// ApprovePR marks an open PR approved (the human path), so it can be merged
// without a reviewer agent.
func (c *HTTP) ApprovePR(id string) error {
	return c.post("/pr/approve", api.NameReq{Name: id})
}

// ScrapPR discards a PR alongside scrapping/closing its task (the human path): it
// deletes the task's branch and flips the PR to "scrapped" so it drops off the board.
// The paired task close frees the agent, so this does not.
func (c *HTTP) ScrapPR(id string) error {
	return c.post("/pr/scrap", api.NameReq{Name: id})
}

// DiscardPR scraps a PR ON ITS OWN — for work that simply isn't wanted, with no task being
// closed alongside it. Use this rather than ScrapPR whenever the PR is the only thing going
// away: it also releases the author, which would otherwise wait for a verdict forever.
func (c *HTTP) DiscardPR(id string) error {
	return c.post("/pr/discard", api.NameReq{Name: id})
}

// LintPR asks for the gate's verdict on the commit a PR's branch names: the stored one when that
// commit has it, otherwise a queued check and where it sits. Never the author's working tree.
func (c *HTTP) LintPR(id string) (string, error) {
	var ok struct {
		Out string `json:"ok"`
	}
	return ok.Out, c.get("/pr/lint?id="+url.QueryEscape(id), &ok)
}

// Runs lists all queued and finished runs, fleet-wide.
func (c *HTTP) Runs() ([]api.Run, error) {
	var out []api.Run
	return out, c.get("/runs", &out)
}

// RunInfo returns a run with its stored output.
func (c *HTTP) RunInfo(id string) (api.RunDetail, error) {
	var d api.RunDetail
	return d, c.get("/run?id="+url.QueryEscape(id), &d)
}

// ScheduleRun queues a run the user asked for, against an agent's workspace or — with agent
// empty — the repo's own checkout, and returns it with its place in the queue.
func (c *HTTP) ScheduleRun(command, agent, priority, timeout string) (api.Run, error) {
	var r api.Run
	return r, c.postResult("/run/new", api.ScheduleRunReq{
		Command: command, Agent: agent, Priority: priority, Timeout: timeout,
	}, &r)
}

// CancelRun withdraws a queued or running run.
func (c *HTTP) CancelRun(id string) error {
	return c.post("/run/cancel", api.NameReq{Name: id})
}

// ReprioritiseRun moves a queued run within the queue.
func (c *HTTP) ReprioritiseRun(id, priority string) error {
	return c.post("/run/priority", api.RunPriorityReq{ID: id, Priority: priority})
}

// RequestReview attaches a review requirement to a PR and dispatches it to a
// reviewer agent.
func (c *HTTP) RequestReview(id, requirement string) error {
	return c.post("/pr/review", api.RejectReq{ID: id, Feedback: requirement})
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
func (c *HTTP) Tasks() ([]api.Task, error) {
	var out []api.Task
	return out, c.get("/tasks", &out)
}

// TaskInfo returns one task (refreshed first).
func (c *HTTP) TaskInfo(id string) (api.Task, error) {
	var t api.Task
	return t, c.get("/task?id="+url.QueryEscape(id), &t)
}

// ReconcileTasks repairs stale task statuses against reality (a startup sweep).
func (c *HTTP) ReconcileTasks() error {
	return c.post("/tasks/reconcile", struct{}{})
}

// RefreshTaskComments forces a re-sync of one task's comments from its source
// (the [r]efresh key), bypassing the TTL.
func (c *HTTP) RefreshTaskComments(id string) error {
	return c.post("/task/comments/refresh", api.NameReq{Name: id})
}

// AddTaskComment comments on a task as the human. A GitHub issue receives it upstream and the
// thread is re-read; every other kind of task keeps its thread in the hub.
func (c *HTTP) AddTaskComment(id, body string) error {
	return c.post("/task/comments/add", api.TellReq{Name: id, Msg: body, Source: "user"})
}

// CreateTask creates a task from a spec and returns its id.
func (c *HTTP) CreateTask(s api.TaskSpec) (string, error) {
	var ok struct {
		ID string `json:"ok"`
	}
	err := c.postResult("/tasks", specReq("", s), &ok)
	return ok.ID, err
}

// EditTask applies a spec to an existing task.
func (c *HTTP) EditTask(id string, s api.TaskSpec) error {
	return c.post("/task/edit", specReq(id, s))
}

func specReq(id string, s api.TaskSpec) api.TaskReq {
	return api.TaskReq{ID: id, Title: s.Title, Type: s.Type, Priority: s.Priority, Parent: s.Parent, Description: s.Description, Labels: s.Labels}
}

// SetPriority assigns a task's priority (P-code) — to td or our own db. scope carries the rating to
// the open tasks below it (api.ScopeTask for the named task alone).
func (c *HTTP) SetPriority(id, priority string, scope api.PriorityScope) error {
	return c.post("/priority", api.PriorityReq{ID: id, Priority: priority, Scope: string(scope)})
}

// ApproveTask clears the approval gate on a planner-proposed task; subtree carries the verdict to
// every task below it that still awaits one.
func (c *HTTP) ApproveTask(id string, subtree bool) error {
	return c.post("/task/approve", api.ApproveTaskReq{ID: id, Subtree: subtree})
}

// RejectTask rejects a planner-proposed task with a comment.
func (c *HTTP) RejectTask(id, comment string) error {
	return c.post("/task/reject", api.RejectReq{ID: id, Feedback: comment})
}

// UnassignTask releases a task back to the backlog (refused if a live agent holds it).
func (c *HTTP) UnassignTask(id string) error {
	return c.post("/task/unassign", api.RejectReq{ID: id})
}

// CloseTask marks a task done from the task list (the "done" close). The hub
// dispatches to the task's backend (td close / openspec archive / issue close).
func (c *HTTP) CloseTask(id string) error {
	return c.post("/task/close", api.RejectReq{ID: id})
}

// ReopenTask restores a closed sindri-owned task to "open", with a required reason recorded as a
// task comment. Refused for a task whose status comes from its own source (an openspec change,
// a GitHub issue) — reopen those there.
func (c *HTTP) ReopenTask(id, reason string) error {
	return c.post("/task/reopen", api.RejectReq{ID: id, Feedback: reason})
}

// ScrapTask scraps a task from the task list (the "discard" close). The hub dispatches
// to the backend (td delete / openspec change-dir removal / issue delete). subtree takes
// the task's children with it; withPRs takes the open PR of everything it scraps.
func (c *HTTP) ScrapTask(id string, subtree, withPRs bool) error {
	return c.post("/task/delete", api.ScrapTaskReq{ID: id, Subtree: subtree, PRs: withPRs})
}

// Refresh asks the hub to re-sync tasks from the source of truth.
func (c *HTTP) Refresh() error { return c.post("/refresh", struct{}{}) }

// Log fetches an agent's recent activity-log entries (the timeline).
func (c *HTTP) Log(name string) ([]api.Event, error) {
	var out []api.Event
	return out, c.get("/log?agent="+url.QueryEscape(name), &out)
}

// Repos lists every registered repo (the registry overview / TUI switcher source).
func (c *HTTP) Repos() ([]api.RepoSummary, error) {
	var out []api.RepoSummary
	return out, c.get("/repos", &out)
}

// RepoInfo returns one repo's resolved config + counts; an empty tag defaults to the
// caller's repo (the client's X-Sindri-Project).
func (c *HTTP) RepoInfo(tag string) (api.RepoDetail, error) {
	var out api.RepoDetail
	path := "/repo"
	if tag != "" {
		path += "?tag=" + url.QueryEscape(tag)
	}
	return out, c.get(path, &out)
}

// RepoInit registers the caller's repo and scaffolds its .sindri/config.yaml.
func (c *HTTP) RepoInit() (api.RepoSummary, error) {
	var out api.RepoSummary
	return out, c.postResult("/repo/init", struct{}{}, &out)
}

// RepoForget drops a repo from the registry by tag (files untouched, agent-guarded).
func (c *HTTP) RepoForget(tag string) error {
	return c.post("/repo/forget", api.RepoReq{Tag: tag})
}

// SetRepoColor pins a repo's colour choice by tag (0 = hash-derived default).
func (c *HTTP) SetRepoColor(tag string, color int) error {
	return c.post("/repo/color", api.RepoReq{Tag: tag, Color: color})
}

// RemoveOrphan removes a stray container by name (a running pod with no roster entry).
func (c *HTTP) RemoveOrphan(name string) error {
	return c.post("/orphan/remove", api.NameReq{Name: name})
}

// WriteRepoConfig persists the caller's repo's .sindri/config.yaml (validated hub-side).
func (c *HTTP) WriteRepoConfig(cfg config.Config) error {
	return c.post("/repo/config", cfg)
}
