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
	"strings"
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

// Archive marks a change done: `openspec archive <name> --yes` moves it out of the
// active set and folds its deltas into the main specs. This is the "done" close for
// an openspec item. A CLI failure is surfaced.
func Archive(projectRoot, name string) error {
	cmd := exec.Command("openspec", "archive", name, "--yes")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("openspec archive %s: %s", name, lastLine(string(out)))
	}
	return nil
}

// DeleteChange scraps a change: it removes the change's proposal directory
// (openspec/changes/<name>) without touching the main specs — the "scrap" close.
// The dir is git-tracked, so a mistaken scrap is recoverable with git. The name is
// validated to a single path segment so it can't escape the changes dir.
func DeleteChange(projectRoot, name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return fmt.Errorf("invalid change name %q", name)
	}
	dir := filepath.Join(projectRoot, "openspec", "changes", name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("no such change %q at %s", name, dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("scrap change %s: %w", name, err)
	}
	return nil
}

// lastLine returns the last non-empty line of s (an openspec error is usually its
// final line), so a failure surfaces the reason, not the whole output.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return strings.TrimSpace(s)
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
