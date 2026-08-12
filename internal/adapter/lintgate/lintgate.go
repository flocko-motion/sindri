// package: adapter/lintgate / lintgate
// type:    adapter (external tool: brokkr)
// job:     runs `brokkr lint` in a worktree, the built-in half of the submit gate —
// single implementation, imported directly by rule, but its own adapter
// package so no logic package shells out to the binary itself.
// limits:  runs the binary and reports its output; resolving its path is the
// caller's (-> hub/repo.Gate, which also runs the project's own verify).
package lintgate

import (
	"os/exec"

	"github.com/flo-at/sindri/internal/adapter/gate"
)

// Adapter runs `brokkr lint`, resolving the binary's path however the caller wants — the hub
// locates its own toolbelt binary; a test can stub ResolveBin to point at a fake one. Also a
// gate.Gate, the same seam openspec validation plugs into.
type Adapter struct {
	ResolveBin func() (string, error)
}

var _ gate.Gate = Adapter{}

// Validate runs `brokkr lint` in wt; ok=false means either the binary couldn't be resolved or the
// lint itself failed — output carries which.
func (a Adapter) Validate(wt string) (ok bool, output string) {
	bin, err := a.ResolveBin()
	if err != nil {
		return false, "lint: " + err.Error()
	}
	cmd := exec.Command(bin, "lint")
	cmd.Dir = wt
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}
