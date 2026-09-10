// package: hub/project / config
// type:    logic (what a repo says about itself)
// job:     load a registered repo's .sindri/config.yaml and answer what the hub acts on — the
// architecture-doc path an agent's brief points at, the quality gate a submit needs, and the
// startup advice about a repo that has named neither.
// limits:  reading and reporting. Validation is internal/config's, and containerfile and
// review_prompt are read at their own call sites.
package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
)

// DocState is a repo's architecture-doc situation; it crosses the wire, so it is
// internal/api.RepoDocState under the name every existing caller here already uses.
type DocState = api.RepoDocState

// root resolves a project tag to its on-disk repo root, "" when the registry has no such tag.
func (s *Service) root(project string) string {
	rows, err := s.store.Projects()
	if err != nil {
		return ""
	}
	for _, p := range rows {
		if p.Tag == project {
			return p.Path
		}
	}
	return ""
}

// Config is a project's resolved config (repo file over defaults). A config error fails the
// operation that needs the project, loudly — an invalid config is surfaced, never ignored.
func (s *Service) Config(project string) (config.Config, error) {
	return config.Load(s.root(project))
}

// ArchitectureDoc is the project's configured architecture-doc path (repo-relative), falling back
// to the default if the config is unreadable — launch already gates on a valid config, so this is
// belt-and-braces for the review-instruction path.
func (s *Service) ArchitectureDoc(project string) string {
	if cfg, err := s.Config(project); err == nil && cfg.Architecture != "" {
		return cfg.Architecture
	}
	return "ARCHITECTURE.md"
}

// StartupAdvice says once, per repo, what the user should know: a config that won't load (which
// otherwise surfaces days later, at the first operation needing that repo), and a missing
// architecture doc, which breaks nothing but leaves agents with no brief. Recommending is the hub's
// business; it used to SEED a placeholder ARCHITECTURE.md, which littered repos that never wanted one.
func (s *Service) StartupAdvice() []string {
	rows, err := s.store.Projects()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range rows {
		if p.Tag == api.GlobalProject {
			continue // no repo, no doc to advise on
		}
		if st := s.DocState(p.Path); st.Advice != "" {
			out = append(out, fmt.Sprintf("%s: %s", filepath.Base(p.Path), st.Advice))
		}
	}
	return out
}

// DocState resolves one repo's architecture-doc situation — the single place the rule lives, so
// the hub's startup line and the TUI's Repos detail can't drift apart.
func (s *Service) DocState(root string) DocState {
	cfg, err := config.Load(root)
	if err != nil {
		// Includes a configured doc that isn't there: validate rejects a named path whose
		// target is missing, so the error already names it. Reported here because otherwise
		// a broken config stays silent until the first operation that needs this repo.
		return DocState{Advice: err.Error()}
	}
	st := DocState{Doc: cfg.Architecture, Set: cfg.ArchitectureSet}
	st.Gate, st.GateOK, st.GateAdvice = gateState(root, cfg.Verify)
	if _, serr := os.Stat(filepath.Join(root, st.Doc)); serr == nil {
		st.Readable = true
		return st
	}
	st.Advice = fmt.Sprintf("no architecture doc — agents get no architecture brief. Point sindri at yours with `architecture: <path>` in %s/.sindri/config.yaml", root)
	return st
}

// gateState resolves the repo's quality gate the same way, and for the same reason: every surface
// that reports it reads this, so the TUI, the CLI and the hub's own warning agree.
func gateState(root, verify string) (gate string, ok bool, advice string) {
	if verify == "" {
		return "", false, fmt.Sprintf("NO QUALITY GATE — nothing can be submitted from this repo. Set `verify:` in "+
			"%s/.sindri/config.yaml to the command that builds, tests and lints it; `verify: make check` is the "+
			"usual answer, and any shell command does", root)
	}
	// Declared is configured. Statting it as a repo-relative file called `make check` a missing
	// script: the gate is a COMMAND, and only running it answers whether it works (-> repo.Gate).
	return verify, true, ""
}
