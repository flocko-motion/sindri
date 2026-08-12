// package: hub / reqscope
// type:    logic (which repo a host request concerns)
// job:     derive a request's project — the header the client sends, the routes exempt
// from needing one, and the agent-name lookup that spans repos because the
// roster the front-ends show is fleet-wide.
// limits:  resolution only; the endpoints themselves are server.go's.
package hub

import (
	"fmt"
	"net/http"
)

// globalRoutes are the only control endpoints valid without a repo context: the
// board reads, which return global agents/PRs (and no tasks when no repo is
// selected). Everything else is repo-scoped and requires X-Sindri-Project.
var globalRoutes = map[string]bool{
	"/state": true, "/events": true, "/stats": true,
	// Registry management spans repos: listing, inspecting, and forgetting a repo
	// operate on the registry by tag, not on the caller's cwd.
	"/repos": true, "/repo": true, "/repo/forget": true, "/repo/color": true,
	// Orphan removal targets a container by its (globally-unique) name, not a repo.
	"/orphan/remove": true,
}

// requireProject rejects a repo-scoped request that arrives without an
// X-Sindri-Project header (rather than silently acting on a phantom empty project),
// with a clear message. The board reads are exempt (see globalRoutes).
func requireProject(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !globalRoutes[r.URL.Path] && r.Header.Get("X-Sindri-Project") == "" {
			writeJSON(w, nil, fmt.Errorf("missing repo context (X-Sindri-Project) — run this inside a repo"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// reqProject resolves (and registers) the repo a host request concerns, from the
// X-Sindri-Project header (the client sends its repo root). Returns the repoTag; ""
// when no header is present (a repo-agnostic request, e.g. the board with no repo
// selected). This is the single place a host request's project is derived.
func (h *Hub) reqProject(r *http.Request) string {
	root := r.Header.Get("X-Sindri-Project")
	if root == "" {
		return ""
	}
	h.repo(root) // register (idempotent) + ensure .worktrees gitignore
	return repoTag(root)
}

// agentReq resolves the project of the agent a request names. The roster is FLEET-WIDE — both
// front-ends list every repo's agents — while a request's project comes from the caller's cwd, so
// stopping an agent of another repo answered that it does not exist. The caller's own repo wins any
// name clash, the same precedence PRProject gives ids.
func (h *Hub) agentReq(r *http.Request, name string) string {
	own := h.reqProject(r)
	if own != "" {
		if _, ok, _ := h.store.For(own).GetAgent(name); ok {
			return own
		}
	}
	agents, err := h.store.AllAgents()
	if err != nil {
		return own
	}
	for _, a := range agents {
		if a.Name == name {
			return a.Project
		}
	}
	return own
}
