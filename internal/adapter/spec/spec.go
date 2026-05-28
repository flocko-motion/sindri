// Package spec adapts the external openspec CLI to sindri. It provides a
// read-only view of a project's openspec/ directory (changes and specs) by
// shelling out to `openspec list --json`.
//
// It only reads, for display in the work list; the spec workflow itself
// (propose/apply/archive) is driven by the openspec CLI inside agent
// containers, not here. Assembly into issues lives in internal/board.
package spec

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

// Capability is an established spec from `openspec list --specs --json`.
type Capability struct {
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

// Capabilities returns established specs via `openspec list --specs --json`.
func Capabilities(projectRoot string) []Capability {
	if !Enabled(projectRoot) {
		return nil
	}
	out, err := run(projectRoot, "list", "--specs", "--json")
	if err != nil {
		return nil
	}
	var result struct {
		Specs []Capability `json:"specs"`
	}
	if json.Unmarshal(out, &result) != nil {
		return nil // plain "No specs found." or CLI missing
	}
	return result.Specs
}

func run(projectRoot string, args ...string) ([]byte, error) {
	cmd := exec.Command("openspec", args...)
	cmd.Dir = projectRoot
	return cmd.Output()
}
