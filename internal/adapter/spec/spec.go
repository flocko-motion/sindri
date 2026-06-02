// package: spec
// type:    adapter (external tool)
// job:     wraps the openspec CLI — reads a project's changes via
//          `openspec list --json` and validates specs via `openspec validate`.
// limits:  read-only; the propose/apply/archive workflow runs via the openspec
//          CLI in agent containers, not here. Doesn't assemble (-> board).
package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Change is a proposed change from `openspec list --json`.
type Change struct {
	Name           string `json:"name"`
	CompletedTasks int    `json:"completedTasks"`
	TotalTasks     int    `json:"totalTasks"`
	LastModified   string `json:"lastModified"`
	Status         string `json:"status"`
}

// Enabled reports whether the project uses openspec (has an openspec/ dir).
func Enabled(projectRoot string) bool {
	info, err := os.Stat(filepath.Join(projectRoot, "openspec"))
	return err == nil && info.IsDir()
}

// Changes returns proposed changes via `openspec list --json`.
// Returns nil if openspec is not enabled or the CLI is unavailable.
func Changes(projectRoot string) []Change {
	if !Enabled(projectRoot) {
		return nil
	}
	out, err := run(projectRoot, "list", "--json")
	if err != nil {
		return nil
	}
	var result struct {
		Changes []Change `json:"changes"`
	}
	if json.Unmarshal(out, &result) != nil {
		return nil
	}
	return result.Changes
}

// Lookup returns the active change with the given name, or nil if no active
// change carries that name (it may not exist at all, or it may have been
// archived). Use IsArchived to tell those two apart when it matters.
func Lookup(projectRoot, name string) *Change {
	for _, c := range Changes(projectRoot) {
		if c.Name == name {
			cc := c
			return &cc
		}
	}
	return nil
}

// Archive runs `openspec archive <name> --yes`, moving the proposal under
// openspec/changes/archive/ and applying its deltas to specs/.
func Archive(projectRoot, name string) error {
	_, err := run(projectRoot, "archive", name, "--yes")
	return err
}

// Abandon deletes the change folder so the proposal is dropped without being
// applied. It refuses if the change isn't active — there's nothing to abandon
// once archived, and we never delete the archive folder. Returns the path
// that was removed so callers can include it in their confirmation.
func Abandon(projectRoot, name string) (string, error) {
	p := filepath.Join(projectRoot, "openspec", "changes", name)
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("spec %s: not found at %s (already archived?)", name, p)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("spec %s: %s is not a directory", name, p)
	}
	if err := os.RemoveAll(p); err != nil {
		return "", fmt.Errorf("spec %s: remove: %w", name, err)
	}
	return p, nil
}

// Validate runs `openspec validate --all` for the project. It degrades
// gracefully: when openspec isn't used (no openspec/ dir) or the CLI isn't
// installed, it reports ok=true with no output, so non-openspec projects and
// missing tooling never block a lint or submit. ok=false means at least one
// spec is invalid; output carries the validator's report.
func Validate(projectRoot string) (ok bool, output string) {
	if !Enabled(projectRoot) {
		return true, ""
	}
	cmd := exec.Command("openspec", "validate", "--all")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		// A non-zero exit means validation failures; anything else (e.g. the
		// CLI not being installed) is treated as "skip", not a hard failure.
		if _, isExit := err.(*exec.ExitError); isExit {
			return false, string(out)
		}
		return true, ""
	}
	return true, string(out)
}

func run(projectRoot string, args ...string) ([]byte, error) {
	cmd := exec.Command("openspec", args...)
	cmd.Dir = projectRoot
	return cmd.Output()
}
