// package: adapter/gh / gh
// type:    adapter (external tool: gh)
// job:     the thin low-level wrapper around the gh CLI: is it installed, and run
// `gh api <path>`. adapter/tasks/github builds the task-source domain on top;
// update calls straight through, so importing it never drags hub/task in.
// limits:  no issue/task semantics — that domain lives in adapter/tasks/github.
package gh

import (
	"context"
	"os/exec"
	"strings"
)

// Installed reports whether the gh CLI is on PATH.
func Installed() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// API runs `gh api <path>` and returns its raw stdout — the general-purpose call every caller
// building on gh's own auth goes through, rather than a second call site shelling out itself.
// stderr comes back alongside a non-nil err so a caller can tell e.g. a 404 from a real failure.
func API(ctx context.Context, path string) (stdout []byte, stderr string, err error) {
	cmd := exec.CommandContext(ctx, "gh", "api", path)
	var out, errBuf strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err = cmd.Run()
	return []byte(out.String()), strings.TrimSpace(errBuf.String()), err
}
