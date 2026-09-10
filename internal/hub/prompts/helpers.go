// package: hub/prompts / helpers
// type:    logic (the small renderings the agent-facing strings are built from)
// job:     the pieces several prompts share — the paths an agent is told about, a placeholder for
// an empty field, and the block that lists what arrived on a branch.
// limits:  rendering. Nothing here reads anything.
package prompts

import (
	"fmt"
	"strings"
)

// ScratchMount is where the scratch worktree appears in the pod. Named for what it is: disposable,
// and not where work is meant to live.
const ScratchMount = "/scratch"

// AgentTrees is the in-repo directory holding every agent's worktree. Named here because the
// coauthor's pod HIDES it (-> hub/agent/lifecycle.go) while mounting one tree inside it, and two
// spellings would hide the wrong path or mount nothing.
const AgentTrees = ".worktrees"

// RefName is how the reference branch is spoken of to an agent. Its real name is the hub's
// business: an agent works on "your branch" against "the reference branch" and needs no more,
// and a name it never learns is a name it cannot try to check out.
const RefName = "the reference branch"

// dash renders "-" for an empty string, so a prompt naming a field never trails off mid-sentence.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// commitList renders incoming commits as an indented block, or "" when there are none to name.
func commitList(incoming []string) string {
	if len(incoming) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nWhat arrived (%d):\n", len(incoming))
	for _, l := range incoming {
		b.WriteString("  " + l + "\n")
	}
	return b.String()
}

// ShortSHA names a commit in a reply: enough to identify it, short enough to read.
func ShortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
