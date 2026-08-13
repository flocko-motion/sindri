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
	"os"
	"os/exec"
	"path/filepath"
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

// Run serves the Go tools for dir, delegating a healthy module to `gopls mcp` rather than proxying
// it. Everything it cannot serve it EXPLAINS: the server is declared for every pod now, so this is
// the only thing that can tell a tree apart, and an empty answer would read as a fact about code
// nothing examined.
func Run(dir string, in io.Reader, out io.Writer, stderr io.Writer) error {
	root, found := ModuleRoot(dir)
	if !found {
		return refuse(NoModuleAdvice(dir), in, out, stderr)
	}
	if advice := Probe(root); advice != "" {
		return refuse(advice, in, out, stderr)
	}
	cmd := exec.Command("gopls", "mcp")
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = root, in, out, stderr
	return cmd.Run()
}

// refuse serves the diagnosis to the client and states it on stderr, where a launch log keeps it.
func refuse(advice string, in io.Reader, out io.Writer, stderr io.Writer) error {
	fmt.Fprintln(stderr, advice)
	return serveAdvice(advice, in, out)
}

// searchDepth bounds the hunt for a module below dir: deep enough for the usual layouts, shallow
// enough to stay a glance rather than a walk of somebody's node_modules.
const searchDepth = 3

// skipDirs never contain the module being served, and are where a deep walk goes to die.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".worktrees": true, "testdata": true,
}

// ModuleRoot is where to serve Go tools from: dir when it holds a go.work or go.mod, else the
// shallowest module below. go.work wins outright — a tree of several modules is exactly the shape a
// single root go.mod test called "not Go".
func ModuleRoot(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	for _, name := range []string{"go.work", "go.mod"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return dir, true
		}
	}
	return shallowestModule(dir)
}

// shallowestModule breadth-first-searches for a go.mod, so the outermost module wins rather than
// whichever the walk happened to reach first.
func shallowestModule(dir string) (string, bool) {
	level := []string{dir}
	for depth := 0; depth < searchDepth && len(level) > 0; depth++ {
		var next []string
		for _, d := range level {
			entries, err := os.ReadDir(d)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if skipDirs[e.Name()] || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				child := filepath.Join(d, e.Name())
				if _, err := os.Stat(filepath.Join(child, "go.mod")); err == nil {
					return child, true
				}
				next = append(next, child)
			}
		}
		level = next
	}
	return "", false
}

// NoModuleAdvice says what was looked for and where, so the refusal is about THIS tree.
func NoModuleAdvice(dir string) string {
	if dir == "" {
		return "No workspace to serve: the Go tools have nothing to look at. This is not a fact about " +
			"any code — nothing was examined."
	}
	return fmt.Sprintf("No Go module under %s: no go.work or go.mod at the root, and none within %d "+
		"directories below it. The Go tools are unavailable here, which says nothing about the code — "+
		"nothing was examined. If this tree IS Go, its module is deeper than the search or behind a "+
		"skipped directory.", dir, searchDepth)
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
