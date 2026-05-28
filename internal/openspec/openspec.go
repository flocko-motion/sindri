// Package openspec provides a read-only view of a project's openspec/
// directory (spec-driven development) by shelling out to the openspec CLI.
//
// sindri only reads changes/specs for display in the task list. The actual
// spec workflow (propose/apply/archive) is driven by the openspec CLI inside
// agent containers.
package openspec

import (
	"encoding/json"
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

// Spec is an established capability from `openspec list --specs --json`.
type Spec struct {
	Name string `json:"name"`
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
	out, err := runOpenspec(projectRoot, "list", "--json")
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

// Specs returns established specs via `openspec list --specs --json`.
func Specs(projectRoot string) []Spec {
	if !Enabled(projectRoot) {
		return nil
	}
	out, err := runOpenspec(projectRoot, "list", "--specs", "--json")
	if err != nil {
		return nil
	}
	var result struct {
		Specs []Spec `json:"specs"`
	}
	if json.Unmarshal(out, &result) != nil {
		return nil // plain "No specs found." or CLI missing
	}
	return result.Specs
}

func runOpenspec(projectRoot string, args ...string) ([]byte, error) {
	cmd := exec.Command("openspec", args...)
	cmd.Dir = projectRoot
	return cmd.Output()
}
