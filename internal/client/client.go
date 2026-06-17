// package: client / client
// type:    adapter (HTTP over a unix socket)
// job:     the thin client every host-side caller (CLI, TUI) uses to talk to a
//          running hub over its unix socket. Mirrors the hub's operation set so
//          it is interchangeable with an in-process hub.
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

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// HTTP talks to a hub over its repo unix socket.
type HTTP struct {
	hc   *http.Client
	base string
}

// DialSocket returns a client that talks to the hub over a specific unix socket
// — used by the in-pod browser, which dials its own mounted socket.
func DialSocket(socketPath string) *HTTP {
	return &HTTP{
		base: "http://unix",
		hc: &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		}},
	}
}

// Dial returns a client for the hub serving root's control socket.
func Dial(root string) *HTTP { return DialSocket(hub.SocketPath(root)) }

// Close is a no-op (kept so HTTP satisfies the same interface as *hub.Hub).
func (c *HTTP) Close() error { return nil }

// State fetches the whole board (agents, tasks, PRs, orphans).
func (c *HTTP) State() (hub.BoardState, error) {
	var out hub.BoardState
	return out, c.get("/state", &out)
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

// NewAgent registers an agent identity.
func (c *HTTP) NewAgent(name, role string) error {
	return c.post("/agents", hub.AgentReq{Name: name, Role: role})
}

// Launch spins a pod for an existing agent (shell=true runs a bare shell instead
// of Claude — for demos/debugging).
func (c *HTTP) Launch(name string, shell bool) error {
	return c.post("/launch", hub.NameReq{Name: name, Shell: shell})
}

// Tell delivers a provenance-stamped message into an agent's session.
func (c *HTTP) Tell(name, msg, source string) error {
	return c.post("/tell", hub.TellReq{Name: name, Msg: msg, Source: source})
}

// Commands fetches the caller's currently-available command surface (the browser
// menu). Identity is the socket, so no name is sent.
func (c *HTTP) Commands() ([]hub.CmdInfo, error) {
	var out []hub.CmdInfo
	return out, c.get("/commands", &out)
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

// NewTask creates a task and returns its id.
func (c *HTTP) NewTask(title, typ, priority string, labels []string) (string, error) {
	var ok struct {
		ID string `json:"ok"`
	}
	err := c.postResult("/tasks", hub.TaskReq{Title: title, Type: typ, Priority: priority, Labels: labels}, &ok)
	return ok.ID, err
}

// SetPriority assigns a task's priority (P-code) — to td or our own db.
func (c *HTTP) SetPriority(id, priority string) error {
	return c.post("/priority", hub.PriorityReq{ID: id, Priority: priority})
}

// Refresh asks the hub to re-sync tasks from the source of truth.
func (c *HTTP) Refresh() error { return c.post("/refresh", struct{}{}) }

// Log fetches an agent's recent activity-log entries (the timeline).
func (c *HTTP) Log(name string) ([]store.Event, error) {
	var out []store.Event
	return out, c.get("/log?agent="+url.QueryEscape(name), &out)
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
