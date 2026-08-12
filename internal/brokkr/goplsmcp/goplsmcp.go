// package: brokkr / goplsmcp
// type:    logic (MCP shim in front of gopls)
// job:     serve gopls' type-aware Go tools over MCP, and answer a refused Go toolchain with
// the fix — gopls would answer "No symbols found", which reads as a fact about the code.
// limits:  a shim; it analyses nothing. A healthy workspace goes straight to `gopls mcp`.
package goplsmcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/flo-at/sindri/internal/brokkr/lint"
)

// Tools are the tools gopls' MCP server exposes, as reported by its own tools/list. The shim
// answers under these names so a refusal reaches the agent through the tool it actually reached
// for — a server offering nothing would just look like an integration that failed to load.
var Tools = []string{
	"go_diagnostics", "go_file_context", "go_package_api", "go_rename_symbol",
	"go_search", "go_symbol_references", "go_vulncheck", "go_workspace",
}

// Probe reports the toolchain refusal in dir, or "" when the go command is content. `go list -m`
// reads go.mod without loading any packages, so it is the cheapest command that still hits the
// version floor. A directory that is no Go module at all also fails here, but with a different
// message, and ToolchainAdvice returns "" for it — this shim only claims to recognise one thing.
func Probe(dir string) string {
	cmd := exec.Command("go", "list", "-m")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return ""
	}
	return lint.ToolchainAdvice(string(out))
}

// Run serves the Go tools for dir. A healthy workspace is delegated to `gopls mcp` — this shim
// replaces itself with the real server rather than proxying it, so there is no second thing to
// keep in step. Only a refused toolchain is handled here.
func Run(dir string, in io.Reader, out io.Writer, stderr io.Writer) error {
	advice := Probe(dir)
	if advice == "" {
		cmd := exec.Command("gopls", "mcp")
		cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = dir, in, out, stderr
		return cmd.Run()
	}
	fmt.Fprintln(stderr, advice)
	return serveAdvice(advice, in, out)
}

// rpc is the subset of JSON-RPC 2.0 this shim reads and writes. Notifications have no id, and must
// not be answered — a response to one is a protocol error.
type rpc struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
}

// serveAdvice runs a minimal MCP server whose every tool answers with the diagnosis. It has to be a
// real server: a shim that simply exited would show up as an integration that failed to start,
// which says nothing about go.mod and is the opaque failure this exists to prevent.
func serveAdvice(advice string, in io.Reader, out io.Writer) error {
	enc := json.NewEncoder(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // a tools/call argument can be large
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req rpc
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue // not ours to diagnose; a malformed frame is the client's problem
		}
		if len(req.ID) == 0 { // a notification (e.g. notifications/initialized) — no reply
			continue
		}
		resp := rpc{JSONRPC: "2.0", ID: req.ID, Result: reply(req, advice)}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

// reply answers the three methods a client needs before it can call a tool, plus the call itself.
func reply(req rpc, advice string) any {
	switch req.Method {
	case "initialize":
		// Echo the client's protocol version, as gopls does, rather than pinning one here: a
		// version this shim invented would be refused by a client that moved on.
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2024-11-05"
		}
		return map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "gopls (unavailable)", "version": "shim"},
		}
	case "tools/list":
		tools := make([]map[string]any, 0, len(Tools))
		for _, name := range Tools {
			tools = append(tools, map[string]any{
				"name":        name,
				"description": "UNAVAILABLE — " + advice,
				"inputSchema": map[string]any{"type": "object"},
			})
		}
		return map[string]any{"tools": tools}
	case "tools/call":
		// isError is the part that matters: without it a client reads this as a successful
		// answer, which is the same trap as gopls' own "No symbols found".
		return map[string]any{
			"content": []any{map[string]any{"type": "text", "text": advice}},
			"isError": true,
		}
	default:
		return map[string]any{}
	}
}
