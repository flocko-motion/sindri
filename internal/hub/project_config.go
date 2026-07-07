// package: hub / project_config
// type:    logic (per-project config wiring)
// job:     load a project's .sindri/config.yaml and expose what the hub acts on — the
//          architecture-doc path for the reviewer prompt, and ARCHITECTURE.md seeding
//          (only when the project didn't set its own doc). containerfile/review_prompt
//          are read at their own call sites.
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

// ensureArchitectureDoc seeds a placeholder ARCHITECTURE.md at the repo root when none
// exists, so every repo the hub serves gains a home for its architecture rules — the
// file reviewers are told to read before every verdict. Only called when the project
// hasn't configured its own `architecture` path. Idempotent and best-effort: it only
// creates a missing file (never overwrites the project's own doc) and never blocks hub
// startup, but a write error is reported, not swallowed.
func ensureArchitectureDoc(root string) {
	path := filepath.Join(root, "ARCHITECTURE.md")
	if _, err := os.Stat(path); err == nil {
		return // present already — leave the project's doc alone
	} else if !os.IsNotExist(err) {
		return // can't tell (permissions, etc.) — don't risk clobbering
	}
	if err := os.WriteFile(path, []byte(architecturePlaceholder), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "hub: WARNING — could not seed %s: %v\n", path, err)
	}
}
