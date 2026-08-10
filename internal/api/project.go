// package: api / project
// type:    logic (wire types)
// job:     a registered repo as it crosses the wire: the registry row (Project),
// its listing summary (RepoSummary), and the resolved-config-plus-counts
// behind `repo info` (RepoDetail).
// limits:  data only; the registry management lives in hub/project.
package api

// Project is one row of the registry: a repo the hub knows, keyed by its stable
// repoTag (a digest of the abs path), with the on-disk path, when first seen, and
// when last used (touched on every register/use, so the repo switcher can order by
// recency).
type Project struct {
	Tag       string `json:"tag"`
	Path      string `json:"path"`
	FirstSeen string `json:"first_seen"`
	LastUsed  string `json:"last_used"`
	Color     int    `json:"color"` // repo colour choice: 0 = hash-derived default, 1..N = palette index
}

// RepoSummary is one row of the registry overview (`repo list`, the TUI switcher).
type RepoSummary struct {
	Tag           string `json:"tag"`
	Name          string `json:"name"` // repo directory basename
	Path          string `json:"path"`
	Agents        int    `json:"agents"` // roster size (registered agents, not liveness)
	IssuesEnabled bool   `json:"issues_enabled"`
	LastUsed      string `json:"last_used"`
}

// RepoDetail is the resolved config plus counts behind `repo info`.
type RepoDetail struct {
	RepoSummary
	Config    Config `json:"config"`
	OpenTasks int    `json:"open_tasks"`
	Tasks     int    `json:"tasks"`
	OpenPRs   int    `json:"open_prs"`
	PRs       int    `json:"prs"`
}
