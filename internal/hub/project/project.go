// package: hub/project / project
// type:    logic (repo-registry management)
// job:     the management surface over the project registry the hub keeps — list/
// inspect registered repos, additive Init (register + scaffold config),
// Forget (drop the row, agent-guarded, files untouched), config writes,
// orphan removal, display colour. Backs the `repo` CLI/TUI commands.
// limits:  registry + per-repo config only; never deletes a repo's files. Agent
// teardown, .gitignore upkeep, and tag derivation come from the hub via Deps.
package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// orphanRemoveTimeout bounds tearing an orphaned pod down. Wider than a liveness probe: `rm -f`
// stops before it removes, so it is the slowest verb here (-> workflow.runRemoveTimeout).
const orphanRemoveTimeout = 30 * time.Second

// Deps is what registry management needs from the hub: agent teardown, .gitignore upkeep, naming.
type Deps interface {
	DeleteAgent(project, name string) error
	EnsureGitignore(root string)
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

// configTemplate is Init's scaffold: all comments, so it parses as an empty doc until uncommented.
const configTemplate = `# sindri per-project config — see the README "Per-project config" section.
# All keys are optional; repo overrides global overrides the built-in default.

# architecture: docs/ARCHITECTURE.md    # doc the reviewer must read (default: ARCHITECTURE.md)
# containerfile: .sindri/Containerfile  # agent image recipe (highest precedence)
# review_prompt: .sindri/review.md      # file whose contents become the reviewer prompt
# github:
#   issues: false                       # import open GitHub issues as tasks (default: true)
`

// Summary is one row of the registry overview (`repo list`, the TUI switcher). It
// crosses the wire, so it is internal/api.RepoSummary under the name every existing
// caller here already uses.
type Summary = api.RepoSummary

// Detail is the resolved config plus counts behind `repo info`. It crosses the wire,
// so it is internal/api.RepoDetail under the name every existing caller here already
// uses.
type Detail = api.RepoDetail

// taskOpen reports whether a cached task still counts as open. Local: too small for a dependency.
func taskOpen(status string) bool {
	switch status {
	case "closed", "approved", "merged":
		return false
	}
	return true
}

// Known returns the rows AND the error: swallowed, an unreadable registry reads as "no repos" and
// blanked every agent and PR from the board for a frame.
func (s *Service) Known() ([]store.Project, error) { return s.store.Projects() }

// List returns every registered repo with a cheap summary; live-agent ordering is the UI's concern.
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

// summary is best-effort: a broken config reports issues-off rather than failing the whole listing.
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

// Info returns config + counts. Unlike List, a config error IS returned — the user asked to see it.
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
		RepoSummary: s.summary(store.Project{Tag: project, Path: path}),
		Config:      cfg, Tasks: len(tasks), PRs: len(prs),
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

// Init registers a repo and scaffolds config.yaml if absent, never overwriting. Not a precondition:
// an un-init'd repo self-registers on first use. It writes nothing else into the repo.
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
	return s.summary(store.Project{Tag: tag, Path: root}), nil
}

// Forget deletes the repo's agents and its registry row, nothing else: passive data stays keyed by
// the stable tag, so re-adding the repo reactivates it. Hard on agents, soft on records.
func (s *Service) Forget(project string) error {
	if project == api.GlobalProject {
		return fmt.Errorf("%s is not a repo — it takes no worktrees and cannot be forgotten", api.GlobalProject)
	}
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

// WriteConfig persists config.yaml through the hub, the single writer, validating first so a broken
// config is never written.
func (s *Service) WriteConfig(root string, cfg config.Config) error {
	return config.Write(root, cfg)
}

// RemoveOrphan rm's a pod with no roster entry. No identity to delete, and the container name is
// globally unique, so no project context is needed.
func (s *Service) RemoveOrphan(ctx context.Context, name string) error {
	rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), orphanRemoveTimeout)
	defer rmCancel()
	if err := container.RmContext(rmCtx, name); err != nil {
		return fmt.Errorf("remove orphan container %s: %w", name, err)
	}
	s.deps.Notify()
	return nil
}

// SetColor pins a palette index (0 = hash-derived default). A per-machine display preference, so it
// lives in the registry, not the committed config.
func (s *Service) SetColor(project string, color int) error {
	if color < 0 {
		return fmt.Errorf("colour choice must be >= 0 (0 = default), got %d", color)
	}
	return s.store.SetProjectColor(project, color)
}
