// package: hub / repo
// type:    logic (repo-registry management)
// job:     the management surface over the project registry the hub already keeps —
//          list/inspect registered repos, explicit additive `init` (register +
//          scaffold .sindri/config.yaml), and `forget` (drop the registry row only,
//          agent-guarded, files untouched). Backs the `repo` CLI/TUI command set.
// limits:  registry + per-repo config only; never deletes a repo's files or agents.
package hub

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// configTemplate is the commented .sindri/config.yaml scaffolded by `repo init`. It
// is all comments, so it parses as an empty doc (every key defaults) until the user
// uncomments what they want.
const configTemplate = `# sindri per-project config — see the README "Per-project config" section.
# All keys are optional; repo overrides global overrides the built-in default.

# architecture: docs/ARCHITECTURE.md    # doc the reviewer must read (default: ARCHITECTURE.md)
# containerfile: .sindri/Containerfile  # agent image recipe (highest precedence)
# review_prompt: .sindri/review.md      # file whose contents become the reviewer prompt
# github:
#   issues: false                       # import open GitHub issues as tasks (default: true)
`

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
	Config    config.Config `json:"config"`
	OpenTasks int           `json:"open_tasks"`
	Tasks     int           `json:"tasks"`
	OpenPRs   int           `json:"open_prs"`
	PRs       int           `json:"prs"`
}

// RepoList returns every registered repo with a cheap summary (roster size + whether
// the GitHub source is on). Live-agent ordering is a UI concern computed from the
// board, not here.
func (h *Hub) RepoList() ([]RepoSummary, error) {
	projects, err := h.store.Projects()
	if err != nil {
		return nil, err
	}
	out := make([]RepoSummary, 0, len(projects))
	for _, p := range projects {
		out = append(out, h.repoSummary(p))
	}
	return out, nil
}

// repoSummary builds a summary for one registry row (best-effort on roster/config —
// a broken config just reports issues-off rather than failing the whole listing).
func (h *Hub) repoSummary(p store.Project) RepoSummary {
	roster, _ := h.store.For(p.Tag).Roster()
	issues := false
	if cfg, err := config.Load(p.Path); err == nil {
		issues = cfg.IssuesEnabled()
	}
	return RepoSummary{
		Tag: p.Tag, Name: filepath.Base(p.Path), Path: p.Path,
		Agents: len(roster), IssuesEnabled: issues, LastUsed: p.LastUsed,
	}
}

// RepoInfo returns a repo's resolved config and its agent/PR/task counts. A config
// error IS returned here (unlike the listing) — `repo info` is where the user asks
// to see the config, so a broken one must surface, not hide.
func (h *Hub) RepoInfo(project string) (RepoDetail, error) {
	path, ok, err := h.store.ProjectPath(project)
	if err != nil {
		return RepoDetail{}, err
	}
	if !ok {
		return RepoDetail{}, fmt.Errorf("no such repo %q in the registry", project)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return RepoDetail{}, err
	}
	ps := h.store.For(project)
	tasks, _ := ps.AllTasks()
	prs, _ := ps.PRs()
	d := RepoDetail{
		RepoSummary: h.repoSummary(store.Project{Tag: project, Path: path}),
		Config:      cfg, Tasks: len(tasks), PRs: len(prs),
	}
	for _, t := range tasks {
		if !taskDone(t) {
			d.OpenTasks++
		}
	}
	for _, pr := range prs {
		if pr.Status != "merged" {
			d.OpenPRs++
		}
	}
	return d, nil
}

// RepoInit is the additive setup for a repo: register it eagerly, scaffold a
// committed .sindri/config.yaml when absent (never overwriting one), and seed
// ARCHITECTURE.md when the project hasn't configured its own. Idempotent and never a
// precondition — a repo that is never init'd still self-registers on first use.
func (h *Hub) RepoInit(root string) (RepoSummary, error) {
	tag := repoTag(root)
	if err := h.store.RegisterProject(tag, root); err != nil {
		return RepoSummary{}, err
	}
	ensureGitignore(root)
	cfgPath := filepath.Join(root, ".sindri", "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			return RepoSummary{}, fmt.Errorf("create .sindri: %w", err)
		}
		if err := os.WriteFile(cfgPath, []byte(configTemplate), 0o644); err != nil {
			return RepoSummary{}, fmt.Errorf("scaffold %s: %w", cfgPath, err)
		}
	} else if err != nil {
		return RepoSummary{}, fmt.Errorf("stat %s: %w", cfgPath, err)
	}
	if cfg, err := config.Load(root); err == nil && !cfg.ArchitectureSet {
		ensureArchitectureDoc(root)
	}
	return h.repoSummary(store.Project{Tag: tag, Path: root}), nil
}

// RepoForget stops managing a repo: it deletes the repo's agents (freeing their
// pods, worktrees, and identities) and drops the registry row. It does NOT delete
// the repo — its .sindri/ config and git history stay, and all passive project data
// (the task cache, priority overrides, approvals, PRs, and event log) is LEFT in
// place keyed by the repo's stable path-derived tag. So re-adding the same repo
// (repo init / any use) resolves to the same tag and reactivates that data — a soft
// forget for the repo's records, a hard teardown for its running agents.
func (h *Hub) RepoForget(project string) error {
	roster, err := h.store.For(project).Roster()
	if err != nil {
		return err
	}
	for _, a := range roster {
		if err := h.DeleteAgent(project, a.Name); err != nil {
			return fmt.Errorf("forgetting %s: could not delete agent %s: %w", h.repoName(project), a.Name, err)
		}
	}
	return h.store.UnregisterProject(project)
}

// WriteRepoConfig persists a repo's .sindri/config.yaml through the hub (the single
// writer), validating it first so a broken config is never written — the caller
// surfaces the validation error instead.
func (h *Hub) WriteRepoConfig(root string, cfg config.Config) error {
	return config.Write(root, cfg)
}

// RemoveOrphan removes a stray container by name — a pod the board flagged as an
// orphan (running, but with no roster entry). There's no agent identity to delete,
// so this is a direct container rm; the name is a full container name (globally
// unique), so no project context is needed.
func (h *Hub) RemoveOrphan(name string) error {
	if err := container.Rm(name); err != nil {
		return fmt.Errorf("remove orphan container %s: %w", name, err)
	}
	h.notify()
	return nil
}

// SetRepoColor pins a repo's display-colour choice in the registry (0 = the
// hash-derived default; a positive value is a palette index the UI maps to a hue).
// Colour is a per-machine display preference, so it lives in the central registry,
// not the committed .sindri/config.yaml.
func (h *Hub) SetRepoColor(project string, color int) error {
	if color < 0 {
		return fmt.Errorf("colour choice must be >= 0 (0 = default), got %d", color)
	}
	return h.store.SetProjectColor(project, color)
}
