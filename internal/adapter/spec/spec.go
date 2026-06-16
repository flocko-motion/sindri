// package: spec
// type:    adapter (external tool)
// job:     wraps the openspec CLI for the lint gate — detect whether a project
//          uses openspec and validate its specs via `openspec validate`.
// limits:  read-only; the propose/apply/archive workflow runs via the openspec
//          CLI in agent containers, not here.
package spec

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Enabled reports whether the project uses openspec (has an openspec/ dir).
func Enabled(projectRoot string) bool {
	info, err := os.Stat(filepath.Join(projectRoot, "openspec"))
	return err == nil && info.IsDir()
}

// Validate runs `openspec validate --all`. A non-zero exit is a validation
// failure; anything else (e.g. the CLI not installed) is treated as "skip".
func Validate(projectRoot string) (ok bool, output string) {
	if !Enabled(projectRoot) {
		return true, ""
	}
	cmd := exec.Command("openspec", "validate", "--all")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		if _, isExit := err.(*exec.ExitError); isExit {
			return false, string(out)
		}
		return true, ""
	}
	return true, string(out)
}
