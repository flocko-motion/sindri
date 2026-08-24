// package: hub/workflow / scratch
// type:    logic (the coauthor's disposable second workspace)
// job:     name the scratch worktree the launcher creates, and serve `scratch <ref|pr-id>` —
// the hub checking a branch, commit or PR out into it so a coauthor can build and test
// work that is not its own without touching anybody's tree.
// limits:  the verb and the location; mounting it is the launcher's (-> hub/agent/lifecycle.go).
package workflow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
)

// ScratchMount is where the scratch worktree appears in the pod. Named for what it is: disposable,
// and not where work is meant to live.
const ScratchMount = "/scratch"

// AgentTrees is the in-repo directory holding every agent's worktree. Named here because the
// coauthor's pod HIDES it (-> hub/agent/lifecycle.go) while mounting one tree inside it, and two
// spellings would hide the wrong path or mount nothing.
const AgentTrees = ".worktrees"

// ScratchWorktree is a coauthor's scratch tree, relative to the repo root. The same convention
// every other agent's workspace follows — and hidden from the coauthor's own /workspace with the
// rest of AgentTrees, so it is reachable only as ScratchMount.
func ScratchWorktree(agent string) string { return filepath.Join(AgentTrees, agent) }

// ScratchHelp is the verb's help: what it takes, and the two facts that surprise otherwise.
const ScratchHelp = "check a branch, commit or pull request out into " + ScratchMount +
	", your own tree to build and test in: scratch <ref|pr-id> [--force]\n" +
	"  Detached, always: the branch is checked out in its author's worktree, and git gives a branch\n" +
	"  to one worktree at a time. Inspection wants a commit anyway.\n" +
	"  --force  discard what is in " + ScratchMount + " first. Without it an edited scratch refuses:\n" +
	"           a half-finished experiment is exactly what lives there."

// CmdScratch checks a ref out into the caller's scratch worktree. A PR id is accepted as well as a
// ref, since "let me look at pr-sd-xxxx" is the common case and resolving it by hand is friction.
func (e *Engine) CmdScratch(c registry.Caller, args []string, out io.Writer) (int, error) {
	force, ref := parseScratchArgs(args)
	if ref == "" {
		fmt.Fprintln(out, ScratchHelp)
		return 2, nil
	}
	ps := e.store.For(c.Project)
	dir := filepath.Join(e.deps.ProjectRoot(c.Project), ScratchWorktree(c.Agent))
	if _, err := os.Stat(dir); err != nil {
		fmt.Fprintf(out, "you have no %s tree — the hub creates it when your pod starts, so a relaunch gives you one.\n", ScratchMount)
		return 1, nil
	}
	// The branch behind a PR id, so the checkout below is one operation whichever was named.
	target := ref
	pr, ok, err := ps.GetPR(ref)
	if err != nil {
		return 1, err
	}
	if ok {
		target = pr.Branch
	}
	if !force {
		dirty, derr := git.HasChanges(dir)
		if derr != nil {
			return 1, derr
		}
		if dirty {
			fmt.Fprintln(out, ReplyScratchDirty(ref))
			return 1, nil
		}
	}
	// The reviewer checkout, in the reviewer's own live workspace (-> assignReview): detached and
	// clean, in place. NOT MaterializeReview's remove-and-re-add — the pod holds this directory
	// open, so a fresh worktree would leave the container looking at a deleted inode.
	if cerr := git.CheckoutDetachedClean(dir, target); cerr != nil {
		fmt.Fprintf(out, "could not check %s out into %s: %v\n", ref, ScratchMount, cerr)
		return 1, nil
	}
	_ = ps.Log(c.Agent, "scratch", ref+" checked out into "+ScratchMount)
	fmt.Fprintln(out, ReplyScratchReady(ref, target, ScratchMount))
	return 0, nil
}

// parseScratchArgs splits --force from the one ref. Unknown flags fall through as the ref and are
// refused by git itself, naming what it could not resolve — which is the same answer either way.
func parseScratchArgs(args []string) (force bool, ref string) {
	for _, a := range args {
		if a == "--force" || a == "-f" {
			force = true
			continue
		}
		if ref == "" {
			ref = strings.TrimSpace(a)
		}
	}
	return force, ref
}

// ReplyScratchDirty refuses a checkout that would discard an edit. Naming the way out matters more
// than the refusal: nothing in the scratch tree is recorded anywhere, so this is the only warning.
func ReplyScratchDirty(ref string) string {
	return fmt.Sprintf("%s has uncommitted changes, and nothing there is recorded anywhere — checking "+
		"%s out would discard them. Move what you want to keep into /workspace (the user's own "+
		"checkout), or run `sindri scratch %s --force` if you mean to throw it away.",
		ScratchMount, ref, ref)
}

// ReplyScratchReady confirms the checkout, and says what the tree is FOR — building and testing —
// since a workspace that takes edits looks like somewhere to work.
func ReplyScratchReady(ref, target, mount string) string {
	what := ref
	if target != ref {
		what = fmt.Sprintf("%s (branch %s)", ref, target)
	}
	return fmt.Sprintf("%s is checked out in %s, detached. Build and test it there; it is disposable, "+
		"so nothing you leave in it is anybody's work, and the next checkout replaces it. Git itself "+
		"does not reach out of %s — the hub runs it for you.", what, mount, mount)
}
