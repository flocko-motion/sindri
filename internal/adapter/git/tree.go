// package: adapter/git / tree
// type:    adapter (external tool: git)
// job:     name a working tree AS IT STANDS — its commit and everything loose on top — so a caller
// holding a name taken earlier can tell whether the tree in front of it is still the same one.
// limits:  reading. What a moved tree MEANS belongs to whoever was looking at it.
package git

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
)

// TreeFingerprint names dir's working tree: its HEAD, the status of everything loose, and the
// content of every tracked change on top. A name rather than a commit, because the tree it
// describes is usually uncommitted and naming it must leave it exactly as it was found.
//
// An untracked file counts by its NAME: `git diff HEAD` cannot see one, so creating and deleting it
// move the fingerprint while editing it does not. The tree a submit describes is tracked work, and
// reading every ignored file to close that gap costs more than the gap does.
func TreeFingerprint(dir string) (string, error) {
	h := sha256.New()
	status, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		return "", fmt.Errorf("git status in %s: %s", dir, gitError(err))
	}
	h.Write(status)
	// An unborn HEAD is a legitimate tree with no commit to name and no diff to take, so the status
	// above is the whole of what there is to fingerprint.
	head, herr := Head(dir)
	if herr != nil {
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	h.Write([]byte(head))
	diff, derr := exec.Command("git", "-C", dir, "diff", "HEAD").Output()
	if derr != nil {
		return "", fmt.Errorf("git diff in %s: %s", dir, gitError(derr))
	}
	h.Write(diff)
	return hex.EncodeToString(h.Sum(nil)), nil
}
