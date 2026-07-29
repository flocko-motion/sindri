// package: ui/cli / shadow
// type:    logic (install-conflict detection)
// job:     warn when a second sindri or brokkr sits elsewhere on PATH than the one running,
//          so a stale copy is named instead of silently deciding behaviour.
// limits:  reports only; it never removes or reorders anything.
package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// warnShadowedInstall reports a second install of sindri or brokkr found on PATH somewhere other
// than beside the running binary.
//
// Two copies is the failure this exists for: whichever PATH reaches first wins, the hub mounts
// tools from beside ITSELF, and the two can be weeks apart — an afternoon goes into a fix that
// "didn't take". ~/.local/bin is the single install location, so anything else is a leftover.
func warnShadowedInstall(w io.Writer) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	selfDir := filepath.Dir(resolve(self))
	for _, name := range []string{"sindri", "brokkr"} {
		found, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		dir := filepath.Dir(resolve(found))
		if dir == selfDir {
			continue
		}
		fmt.Fprintf(w, "warning: another %s is ahead of this one on PATH: %s (running: %s)\n",
			name, dir, selfDir)
		fmt.Fprintf(w, "  they can be different builds. Keep one install, in ~/.local/bin.\n")
	}
}

// resolve follows symlinks so two paths to one binary aren't reported as rival installs.
func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
