// package: hub / reqscope
// type:    logic (which repo a host request concerns)
// job:     derive a request's project — the header the client sends, and the agent-name
// lookup that spans repos because the roster the front-ends show is fleet-wide.
// limits:  resolution only; the routes that ask are the front-end's (-> hub/api/frontend),
// and which of them may arrive without a repo is its RequireProject.
package hub

import (
	"net/http"

	"github.com/flo-at/sindri/internal/api"
)

// ReqProject resolves (and registers) the repo a host request concerns, from the
// X-Sindri-Project header (the client sends its repo root). Returns the repoTag; ""
// when no header is present (a repo-agnostic request, e.g. the board with no repo
// selected). This is the single place a host request's project is derived.
func (h *Hub) ReqProject(r *http.Request) string {
	root := r.Header.Get("X-Sindri-Project")
	if root == "" {
		return ""
	}
	if root == api.GlobalProject {
		// Already a tag, not a path — nothing to register or hash, unlike every real repo.
		return api.GlobalProject
	}
	h.repo(root) // register (idempotent) + ensure .worktrees gitignore
	return repoTag(root)
}

// AgentReq resolves the project of the agent a request names. The roster is FLEET-WIDE — both
// front-ends list every repo's agents — while a request's project comes from the caller's cwd, so
// stopping an agent of another repo answered that it does not exist. The caller's own repo wins any
// name clash, the same precedence PRProject gives ids.
func (h *Hub) AgentReq(r *http.Request, name string) string {
	own := h.ReqProject(r)
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
