// package: hub/workflow / gitcmd
// type:    logic (the curated git surface agents get)
// job:     `sindri git <sub>` — run an ALLOWLISTED git action in the caller's own
// workspace and return the result, so an agent can see what it changed and put
// files back without a git of its own.
// limits:  the hub builds every git invocation (-> adapter/git); an agent names an
// action and paths — never flags, refs or branch names. Rebase/merge stay their
// own verbs. Replies say "the reference branch", never which branch that is.
package workflow

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
)

// refName is how the reference branch is spoken of to an agent. Its real name is the hub's
// business: an agent works on "your branch" against "the reference branch" and needs no more,
// and a name it never learns is a name it cannot try to check out.
const refName = "the reference branch"

// GitHelp is the `git` verb's help. The allowlist IS the help — an agent's only way to know what
// git it has is being told. One action per line, no flags: the hub chooses those.
const GitHelp = "run a safe git action on your workspace (the hub runs it for you, and only these):\n" +
	"  git status              what you have changed since the hub last recorded your work\n" +
	"  git diff [<paths>]      those changes, as a diff\n" +
	"  git change [<paths>]    your WHOLE change vs " + refName + " — recorded work included\n" +
	"  git history             the work you have already handed over on your branch\n" +
	"  git incoming            what has landed on " + refName + " that you don't have yet\n" +
	"  git restore <paths>     throw away your unrecorded changes to those paths\n" +
	"  git drop <paths>        remove those paths from your change entirely (recorded work too)"

// diffCap bounds a diff's output. A repo-wide diff runs to tens of thousands of lines, which buries
// the answer and costs the agent its context; the tail is named so nothing looks silently complete.
const diffCap = 400

// logCap bounds a commit listing — enough to see what moved, not a project history.
const logCap = 40

// CmdGit dispatches the allowlisted git actions. The action is matched against a fixed set and the
// hub builds the git command from it: an agent supplies paths, never flags, refs or branch names.
func (e *Engine) CmdGit(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
	a, ok, err := ps.GetAgent(c.Agent)
	if err != nil || !ok {
		return 1, fmt.Errorf("agent %s missing: %v", c.Agent, err)
	}
	wt := filepath.Join(root, a.Workspace)
	if len(args) == 0 {
		fmt.Fprintln(out, GitHelp)
		return 2, nil
	}
	paths, perr := safePaths(args[1:])
	if perr != nil {
		fmt.Fprintln(out, perr.Error())
		return 2, nil
	}
	switch args[0] {
	case "status":
		return e.gitStatus(wt, out)
	case "diff":
		return e.gitDiff(wt, paths, out)
	case "change":
		return e.gitChange(wt, root, paths, out)
	case "history":
		return e.gitCommits(wt, root, false, out)
	case "incoming":
		return e.gitCommits(wt, root, true, out)
	case "restore":
		return e.gitRestore(c, wt, paths, out)
	case "drop":
		return e.gitDrop(c, wt, root, paths, out)
	}
	fmt.Fprintf(out, "`git %s` is not available.\n%s\n", args[0], GitHelp)
	return 2, nil
}

// safePaths validates agent-supplied paths: workspace-relative, no escape, no flags. The hub is the
// gatekeeper, so a path is checked here rather than trusted because git would probably refuse it.
func safePaths(paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		if strings.HasPrefix(p, "-") {
			return nil, fmt.Errorf("%q looks like a flag — these actions take paths only, and the hub chooses the flags. Run `sindri git` to see them.", p)
		}
		if filepath.IsAbs(p) {
			return nil, fmt.Errorf("%q is an absolute path — give paths relative to /workspace.", p)
		}
		rel := filepath.Clean(p)
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("%q leads outside your workspace.", p)
		}
		out = append(out, rel)
	}
	return out, nil
}

// gitStatus reports the worktree's own uncommitted changes with git's status codes.
func (e *Engine) gitStatus(wt string, out io.Writer) (int, error) {
	lines, err := git.ChangedNames(wt)
	if err != nil {
		return 1, err
	}
	if len(lines) == 0 {
		fmt.Fprintln(out, "Nothing to show — the hub has already recorded everything in your workspace.")
		return 0, nil
	}
	fmt.Fprintf(out, "%d path(s) changed since the hub last recorded your work (M=modified, A=added, D=deleted, ??=untracked, U=conflicted):\n", len(lines))
	for _, l := range lines {
		fmt.Fprintln(out, "  "+l)
	}
	return 0, nil
}

// gitDiff shows what the agent has changed but not yet committed.
func (e *Engine) gitDiff(wt string, paths []string, out io.Writer) (int, error) {
	diff, err := git.WorkingDiff(wt, paths)
	if err != nil {
		return 1, err
	}
	return writeDiff(out, "changes not yet recorded", paths, diff)
}

// gitChange shows everything the agent's branch introduces over the reference branch — the whole of
// its change, committed work included. What to check before submitting, and what "did I touch files
// this task never needed" is answered from instead of from memory.
func (e *Engine) gitChange(wt, root string, paths []string, out io.Writer) (int, error) {
	ref, err := e.baseBranch(root)
	if err != nil {
		return 1, err
	}
	branch, err := git.CurrentBranch(wt)
	if err != nil {
		return 1, err
	}
	diff, err := git.BranchDiff(wt, ref, branch, paths)
	if err != nil {
		return 1, err
	}
	return writeDiff(out, "whole change vs "+refName, paths, diff)
}

// writeDiff renders a diff reply, bounded, and distinguishes "nothing here" from "nothing at all".
func writeDiff(out io.Writer, what string, paths []string, diff string) (int, error) {
	if strings.TrimSpace(diff) == "" {
		fmt.Fprintf(out, "No %s%s.\n", what, scopeNote(paths))
		return 0, nil
	}
	fmt.Fprintf(out, "Your %s%s:\n", what, scopeNote(paths))
	fmt.Fprint(out, capLines(diff, diffCap))
	return 0, nil
}

// gitCommits lists the agent's own commits, or with incoming what the reference branch has moved on
// by — the answer to "what changed under me", which a rebase otherwise applies unseen.
func (e *Engine) gitCommits(wt, root string, incoming bool, out io.Writer) (int, error) {
	ref, err := e.baseBranch(root)
	if err != nil {
		return 1, err
	}
	branch, err := git.CurrentBranch(wt)
	if err != nil {
		return 1, err
	}
	from, to := ref, branch
	label, empty := "Recorded on your branch", "Nothing of yours is recorded yet — everything you have done is still loose in /workspace."
	if incoming {
		from, to = branch, ref
		label = "On " + refName + ", not yet in your branch"
		empty = "Nothing new on " + refName + " — you already have all of it."
	}
	lines, err := git.LogRange(wt, from, to, logCap)
	if err != nil {
		return 1, err
	}
	if len(lines) == 0 {
		fmt.Fprintln(out, empty)
		return 0, nil
	}
	fmt.Fprintf(out, "%s (%d):\n", label, len(lines))
	for _, l := range lines {
		fmt.Fprintln(out, "  "+l)
	}
	if len(lines) == logCap {
		fmt.Fprintf(out, "  … capped at %d.\n", logCap)
	}
	return 0, nil
}

// gitRestore discards uncommitted changes to paths. Destructive, so it never guesses at scope.
func (e *Engine) gitRestore(c registry.Caller, wt string, paths []string, out io.Writer) (int, error) {
	if len(paths) == 0 {
		fmt.Fprintln(out, "`git restore` needs the paths to put back — it throws work away, so it never guesses. `sindri git status` shows what you changed.")
		return 2, nil
	}
	if err := git.RestoreFromHEAD(wt, paths); err != nil {
		return 1, err
	}
	_ = e.store.For(c.Project).Log(c.Agent, "restore", "to last commit: "+strings.Join(paths, ", "))
	fmt.Fprintf(out, "Threw away your unrecorded changes to %s — they're back as the hub last recorded them. Anything there you hadn't handed over is gone.\n", FileList(paths))
	return 0, nil
}

// gitDrop takes paths out of the agent's change for good: reverting COMMITTED work back to the
// MERGE-BASE (not the reference's current tip, which `git change`'s three-dot diff never measures
// against — dropping against anything else can leave the two disagreeing about the same file) and
// committing that. CommittedChurn and WorktreeDirty are asked separately, each with its own honest
// reply, rather than one worktree-only check that can miss committed churn hidden by a hand-edit.
func (e *Engine) gitDrop(c registry.Caller, wt, root string, paths []string, out io.Writer) (int, error) {
	if len(paths) == 0 {
		fmt.Fprintln(out, "`git drop` needs the paths to remove from your change — it throws work away, so it never guesses. `sindri git change` shows what your change covers.")
		return 2, nil
	}
	ref, err := e.baseBranch(root)
	if err != nil {
		return 1, err
	}
	branch, err := git.CurrentBranch(wt)
	if err != nil {
		return 1, err
	}
	target, err := git.MergeBase(wt, ref, branch)
	if err != nil {
		return 1, err
	}
	churn, err := git.CommittedChurn(wt, target, paths)
	if err != nil {
		return 1, err
	}
	dirty, err := git.WorktreeDirty(wt, paths)
	if err != nil {
		return 1, err
	}
	if !churn && !dirty {
		fmt.Fprintf(out, "%s already match %s exactly — nothing to drop, nothing recorded.\n", FileList(paths), refName)
		return 0, nil
	}
	if err := git.RestoreFromRef(wt, target, paths); err != nil {
		return 1, err
	}
	if !churn {
		// The worktree was dirty but the branch's own commits already agreed with the reference —
		// the edit is cleared, but there was never anything here for `git change` to disagree about.
		fmt.Fprintf(out, "%s already matched %s in your commits — cleared the uncommitted edit there, but there was nothing to record.\n", FileList(paths), refName)
		return 0, nil
	}
	ps := e.store.For(c.Project)
	st, _ := ps.GetState(c.Agent)
	id := st.Task
	if id == "" {
		id = st.Container
	}
	tk, _, _ := ps.GetTask(id)
	desc := "drop " + strings.Join(paths, ", ") + " from this change"
	if err := git.CommitAll(wt, conventionalCommit(tk.Type, id, desc)); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "drop", strings.Join(paths, ", ")+" (restored to "+target+")")
	fmt.Fprintf(out, "Removed %s from your change and recorded that, so those files now match %s exactly. Your work in every other file is untouched.\n", FileList(paths), refName)
	return 0, nil
}

// scopeNote names the path filter in a reply, so an empty result reads as "none HERE" rather than
// "none at all" — an agent that mis-scoped a query must not conclude it changed nothing.
func scopeNote(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return " in " + FileList(paths)
}

// capLines truncates long output and says so, naming what was cut and how to narrow it. Silent
// truncation would read as a complete answer, which for a diff is a wrong answer.
func capLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return strings.Join(lines, "\n") + "\n"
	}
	return fmt.Sprintf("%s\n… truncated: %d of %d lines shown. Narrow it by naming paths.\n",
		strings.Join(lines[:max], "\n"), max, len(lines))
}
