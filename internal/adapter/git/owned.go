// package: adapter/git / owned
// job:     answer whether a worktree is one the hub laid down and may commit in.
// type:    logic (worktree facts)
// limits:  the reading. What is DONE with such a tree is the caller's.
package git

// HubOwnedTree reports a worktree the hub may write into: an agent's own, attached to its own branch.
// A coauthor's IS the user's checkout (RebaseAgent refuses it for the same reason) and a reviewer's is
// a detached look at somebody else's branch — committing in either would write where nobody asked.
func HubOwnedTree(wt, workspace string) bool {
	if workspace == "" || workspace == "." {
		return false
	}
	_, err := CurrentBranch(wt) // errors on a detached HEAD
	return err == nil
}
