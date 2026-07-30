// package: hub / project_config
// type:    logic (per-project config wiring)
// job:     load a project's .sindri/config.yaml and expose what the hub acts on — the
// architecture-doc path for the reviewer prompt, and the startup advice about a
// repo that hasn't named one. containerfile/review_prompt are read at their own
// call sites.
// limits:  thin adapter between internal/config and the hub; validation lives in config.
package hub

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flo-at/sindri/internal/config"
)

// projectConfig loads and validates a project's .sindri/config.yaml (repo over global
// over defaults). A config error fails the operation that needs the project, loudly —
// an invalid config is surfaced, never silently ignored.
func (h *Hub) projectConfig(project string) (config.Config, error) {
	return config.Load(h.projectRoot(project))
}

// architectureDoc is the project's configured architecture-doc path (repo-relative),
// falling back to the default if the config is unreadable — launch already gates on a
// valid config, so this is belt-and-suspenders for the review-instruction path.
func (h *Hub) architectureDoc(project string) string {
	if cfg, err := h.projectConfig(project); err == nil && cfg.Architecture != "" {
		return cfg.Architecture
	}
	return "ARCHITECTURE.md"
}

// StartupAdvice reports what the user should know about each tracked repo, once, when the
// hub starts. It exists because the hub used to SEED a placeholder ARCHITECTURE.md into
// every repo it served, which littered repos that never wanted one — recommending is the
// hub's business, writing into someone's repo is not.
//
// Two things get reported, and a repo in good shape produces neither:
//
//   - A config that won't load. Otherwise this only surfaces at the first operation
//     needing that repo, which may be days later. Note this subsumes an architecture doc
//     that IS configured but absent: config.validate rejects a named path that isn't
//     there, so the error already names it.
//   - No architecture doc at the default path, with none configured. Nothing breaks —
//     SystemPrompt injects nothing for empty content — but agents then work with no
//     architecture brief, which is a quality loss the user should choose knowingly.
func (h *Hub) StartupAdvice() []string {
	projects, err := h.store.Projects()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range projects {
		if st := h.repoDocState(p.Path); st.Advice != "" {
			out = append(out, fmt.Sprintf("%s: %s", filepath.Base(p.Path), st.Advice))
		}
	}
	return out
}

// RepoDocState is a repo's architecture-doc situation: the path in effect and, when the
// hub can't read a doc there, what the user should do about it. Advice is "" for a repo in
// good shape, which is what every UI keys off to stay quiet.
type RepoDocState struct {
	Doc      string `json:"doc"`      // the path in effect: configured, else the default
	Set      bool   `json:"set"`      // the project named it (vs falling back to the default)
	Readable bool   `json:"readable"` // a doc exists at Doc
	Advice   string `json:"advice"`   // "" when nothing to say
}

// repoDocState resolves one repo's architecture-doc situation — the single place the rule
// lives, so the hub's startup line and the TUI's Repos detail can't drift apart.
func (h *Hub) repoDocState(root string) RepoDocState {
	cfg, err := config.Load(root)
	if err != nil {
		// Includes a configured doc that isn't there: validate rejects a named path whose
		// target is missing, so the error already names it. Reported here because otherwise
		// a broken config stays silent until the first operation that needs this repo.
		return RepoDocState{Advice: err.Error()}
	}
	st := RepoDocState{Doc: cfg.Architecture, Set: cfg.ArchitectureSet}
	if _, serr := os.Stat(filepath.Join(root, st.Doc)); serr == nil {
		st.Readable = true
		return st
	}
	st.Advice = fmt.Sprintf("no architecture doc — agents get no architecture brief. Point sindri at yours with `architecture: <path>` in %s/.sindri/config.yaml", root)
	return st
}
