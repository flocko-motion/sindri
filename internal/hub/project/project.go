// package: hub/project / project
// type:    logic (repo-registry management)
// job:     the management surface over the project registry the hub keeps — list/
//          inspect registered repos, additive Init (register + scaffold config),
//          Forget (drop the row, agent-guarded, files untouched), config writes,
//          orphan removal, display colour. Backs the `repo` CLI/TUI commands.
// limits:  registry + per-repo config only; never deletes a repo's files. Agent
//          teardown, path seeding, and tag derivation come from the hub via Deps.
package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// Deps is what registry management needs back from the hub: agent teardown (Forget
// frees a repo's agents), the filesystem seeders, the repo's display name and stable
// tag, and the board notify.
type Deps interface {
	DeleteAgent(project, name string) error
	EnsureGitignore(root string)
	EnsureArchitectureDoc(root string)
	RepoName(project string) string
	RepoTag(root string) string
	Notify()
}

// Service manages the project registry over the hub's store.
type Service struct {
	store *store.Store
	deps  Deps
}

// New builds the registry-management service over the hub's store + its Deps.
func New(st *store.Store, deps Deps) *Service { return &Service{store: st, deps: deps} }

// configTemplate is the commented .sindri/config.yaml scaffolded by Init. It is all
// comments, so it parses as an empty doc (every key defaults) until the user
// uncomments what they want.
const configTemplate = `# sindri per-project config — see the README "Per-project config" section.
# All keys are optional; repo overrides global overrides the built-in default.

# architecture: docs/ARCHITECTURE.md    # doc the reviewer must read (default: ARCHITECTURE.md)
# containerfile: .sindri/Containerfile  # agent image recipe (highest precedence)
# review_prompt: .sindri/review.md      # file whose contents become the reviewer prompt
# github:
#   issues: false                       # import open GitHub issues as tasks (default: true)
`

// Summary is one row of the registry overview (`repo list`, the TUI switcher).
type Summary struct {
	Tag           string `json:"tag"`
	Name          string `json:"name"` // repo directory basename
	Path          string `json:"path"`
	Agents        int    `json:"agents"` // roster size (registered agents, not liveness)
	IssuesEnabled bool   `json:"issues_enabled"`
	LastUsed      string `json:"last_used"`
}

// Detail is the resolved config plus counts behind `repo info`.
type Detail struct {
	Summary
	Config    config.Config `json:"config"`
	OpenTasks int           `json:"open_tasks"`
	Tasks     int           `json:"tasks"`
	OpenPRs   int           `json:"open_prs"`
	PRs       int           `json:"prs"`
}

// taskOpen reports whether a cached task still counts as open (not done). Kept local:
// a tiny domain predicate, not worth a dependency.
func taskOpen(status string) bool {
	switch status {
	case "closed", "approved", "merged":
		return false
	}
	return true
}

// List returns every registered repo with a cheap summary (roster size + whether the
// GitHub source is on). Live-agent ordering is a UI concern computed from the board.
func (s *Service) List() ([]Summary, error) {
	projects, err := s.store.Projects()
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(projects))
	for _, p := range projects {
		out = append(out, s.summary(p))
	}
	return out, nil
}

// summary builds a summary for one registry row (best-effort on roster/config — a
// broken config just reports issues-off rather than failing the whole listing).
func (s *Service) summary(p store.Project) Summary {
	roster, _ := s.store.For(p.Tag).Roster()
	issues := false
	if cfg, err := config.Load(p.Path); err == nil {
		issues = cfg.IssuesEnabled()
	}
	return Summary{
		Tag: p.Tag, Name: filepath.Base(p.Path), Path: p.Path,
		Agents: len(roster), IssuesEnabled: issues, LastUsed: p.LastUsed,
	}
}

// Info returns a repo's resolved config and its agent/PR/task counts. A config error
// IS returned here (unlike List) — `repo info` is where the user asks to see the
// config, so a broken one must surface, not hide.
func (s *Service) Info(project string) (Detail, error) {
	path, ok, err := s.store.ProjectPath(project)
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{}, fmt.Errorf("no such repo %q in the registry", project)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return Detail{}, err
	}
	ps := s.store.For(project)
	tasks, _ := ps.AllTasks()
	prs, _ := ps.PRs()
	d := Detail{
		Summary: s.summary(store.Project{Tag: project, Path: path}),
		Config:  cfg, Tasks: len(tasks), PRs: len(prs),
	}
	for _, t := range tasks {
		if taskOpen(t.Status) {
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

// Init is the additive setup for a repo: register it eagerly, scaffold a committed
// .sindri/config.yaml when absent (never overwriting one), and seed ARCHITECTURE.md
// when the project hasn't configured its own. Idempotent and never a precondition — a
// repo that is never init'd still self-registers on first use.
func (s *Service) Init(root string) (Summary, error) {
	tag := s.deps.RepoTag(root)
	if err := s.store.RegisterProject(tag, root); err != nil {
		return Summary{}, err
	}
	s.deps.EnsureGitignore(root)
	cfgPath := filepath.Join(root, ".sindri", "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			return Summary{}, fmt.Errorf("create .sindri: %w", err)
		}
		if err := os.WriteFile(cfgPath, []byte(configTemplate), 0o644); err != nil {
			return Summary{}, fmt.Errorf("scaffold %s: %w", cfgPath, err)
		}
	} else if err != nil {
		return Summary{}, fmt.Errorf("stat %s: %w", cfgPath, err)
	}
	if cfg, err := config.Load(root); err == nil && !cfg.ArchitectureSet {
		s.deps.EnsureArchitectureDoc(root)
	}
	return s.summary(store.Project{Tag: tag, Path: root}), nil
}

// Forget stops managing a repo: it deletes the repo's agents (freeing their pods,
// worktrees, and identities) and drops the registry row. It does NOT delete the repo
// — its .sindri/ config and git history stay, and all passive project data (task
// cache, priority overrides, approvals, PRs, event log) is LEFT in place keyed by the
// repo's stable path-derived tag. So re-adding the same repo resolves to the same tag
// and reactivates that data — a soft forget for records, a hard teardown for agents.
func (s *Service) Forget(project string) error {
	roster, err := s.store.For(project).Roster()
	if err != nil {
		return err
	}
	for _, a := range roster {
		if err := s.deps.DeleteAgent(project, a.Name); err != nil {
			return fmt.Errorf("forgetting %s: could not delete agent %s: %w", s.deps.RepoName(project), a.Name, err)
		}
	}
	return s.store.UnregisterProject(project)
}

// WriteConfig persists a repo's .sindri/config.yaml through the hub (the single
// writer), validating it first so a broken config is never written — the caller
// surfaces the validation error instead.
func (s *Service) WriteConfig(root string, cfg config.Config) error {
	return config.Write(root, cfg)
}

// RemoveOrphan removes a stray container by name — a pod the board flagged as an
// orphan (running, but with no roster entry). There's no agent identity to delete, so
// this is a direct container rm; the name is a full container name (globally unique),
// so no project context is needed.
func (s *Service) RemoveOrphan(name string) error {
	if err := container.Rm(name); err != nil {
		return fmt.Errorf("remove orphan container %s: %w", name, err)
	}
	s.deps.Notify()
	return nil
}

// SetColor pins a repo's display-colour choice in the registry (0 = the hash-derived
// default; a positive value is a palette index the UI maps to a hue). Colour is a
// per-machine display preference, so it lives in the central registry, not the
// committed .sindri/config.yaml.
func (s *Service) SetColor(project string, color int) error {
	if color < 0 {
		return fmt.Errorf("colour choice must be >= 0 (0 = default), got %d", color)
	}
	return s.store.SetProjectColor(project, color)
}
