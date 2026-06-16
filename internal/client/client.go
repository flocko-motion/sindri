// package: client / client
// type:    adapter (HTTP over a unix socket)
// job:     the thin client every host-side caller (CLI, TUI) uses to talk to a
//          running hub over its unix socket. Mirrors the hub's operation set so
//          it is interchangeable with an in-process hub.
// limits:  no domain logic; just marshals calls to the hub's HTTP API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/flo-at/sindri/internal/hub"
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

// State fetches the agent roster + live status.
func (c *HTTP) State() ([]hub.AgentState, error) {
	var out []hub.AgentState
	return out, c.get("/state", &out)
}

// NewAgent registers an agent identity.
func (c *HTTP) NewAgent(name, role string) error {
	return c.post("/agents", hub.AgentReq{Name: name, Role: role})
}

// Launch spins a pod for an existing agent.
func (c *HTTP) Launch(name string) error {
	return c.post("/launch", hub.NameReq{Name: name})
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

func (c *HTTP) get(path string, out any) error {
	resp, err := c.hc.Get(c.base + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readResult(resp, out)
}

func (c *HTTP) post(path string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.hc.Post(c.base+path, "application/json", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readResult(resp, nil)
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
