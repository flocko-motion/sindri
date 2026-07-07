// package: spec
// type:    adapter (external tool)
// job:     wraps the openspec CLI for the lint gate — detect whether a project
//          uses openspec and validate its specs via `openspec validate`.
// limits:  read-only; the propose/apply/archive workflow runs via the openspec
//          CLI in agent containers, not here.
package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Change is an openspec change from `openspec list --json`.
type Change struct {
	Name           string `json:"name"`
	CompletedTasks int    `json:"completedTasks"`
	TotalTasks     int    `json:"totalTasks"`
	Status         string `json:"status"`
}

// Done reports whether a change's tasks are all complete.
func (c Change) Done() bool { return c.TotalTasks > 0 && c.CompletedTasks == c.TotalTasks }

// Changes lists the project's active openspec changes. Returns (nil, nil) when
// openspec isn't used (an optional source), but a CLI failure or unparseable output
// is returned as an error — after Enabled() is true, those are real failures, not a
// legitimate "no changes".
func Changes(projectRoot string) ([]Change, error) {
	if !Enabled(projectRoot) {
		return nil, nil
	}
	cmd := exec.Command("openspec", "list", "--json")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("openspec list in %s: %w", projectRoot, err)
	}
	var wrap struct {
		Changes []Change `json:"changes"`
	}
	if e := json.Unmarshal(out, &wrap); e != nil {
		return nil, fmt.Errorf("parse openspec list output: %w", e)
	}
	return wrap.Changes, nil
}

// Enabled reports whether the project uses openspec (has an openspec/ dir).
func Enabled(projectRoot string) bool {
	info, err := os.Stat(filepath.Join(projectRoot, "openspec"))
	return err == nil && info.IsDir()
}

// CLIInstalled reports whether the openspec CLI is available on PATH.
func CLIInstalled() bool {
	_, err := exec.LookPath("openspec")
	return err == nil
}

// Validate runs `openspec validate --all`. A non-zero exit is a validation
// failure (ok=false); everything else is a non-failing skip (ok=true). openspec
// is OPTIONAL: a project with no openspec/ skips silently, but one that uses
// openspec yet lacks the CLI degrades with a visible note (in output) rather than
// vanishing — so a skipped validation is never mistaken for a passed one.
func Validate(projectRoot string) (ok bool, output string) {
	if !Enabled(projectRoot) {
		return true, "" // project doesn't use openspec — nothing to validate
	}
	if _, err := exec.LookPath("openspec"); err != nil {
		return true, "openspec/ present but the openspec CLI is not installed — skipping spec validation (optional)"
	}
	cmd := exec.Command("openspec", "validate", "--all")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		if _, isExit := err.(*exec.ExitError); isExit {
			return false, string(out) // a real validation failure
		}
		return true, "openspec validate could not run: " + err.Error() // degrade, but visibly
	}
	return true, string(out)
}
