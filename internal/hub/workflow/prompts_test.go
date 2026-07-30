package workflow

import (
	"strings"
	"testing"
)

// TestSystemPromptCarriesArchitecture: every role's durable brief has the project
// architecture INJECTED (full content, not just a path) plus a re-read pointer — not
// just the reviewer. An agent can't produce work that fits without knowing how it's
// built. Empty content injects nothing.
func TestSystemPromptCarriesArchitecture(t *testing.T) {
	const archPath = "docs/ARCH.md"
	const archContent = "## Layering\nAdapters never import the hub."
	for _, role := range []string{"worker", "reviewer", "planner", "coauthor"} {
		p := SystemPrompt("eitri", role, archContent, archPath)
		if !strings.Contains(p, archContent) {
			t.Errorf("%s: architecture content not injected:\n%s", role, p)
		}
		if !strings.Contains(p, "/workspace/"+archPath) {
			t.Errorf("%s: missing re-read pointer to /workspace/%s", role, archPath)
		}
		if !strings.Contains(p, "`brokkr`") {
			t.Errorf("%s: brief should always recommend brokkr:\n%s", role, p)
		}
	}
	if p := SystemPrompt("eitri", "worker", "", "ARCHITECTURE.md"); strings.Contains(p, "Project architecture") {
		t.Errorf("empty architecture content should inject no section:\n%s", p)
	}
}

// TestGuardRepliesNameTheRealState: the replies an agent hits when a verb doesn't apply
// must describe its ACTUAL state. The flat "run `sindri` to pick up a task first" was
// true only when idle — a worker whose PR was under review got told to abandon the task
// it was holding, and went round in circles.
func TestGuardRepliesNameTheRealState(t *testing.T) {
	// Holding a task: never tell it to go get one.
	for _, phase := range []string{"submitted", "resolving"} {
		got := ReplyNotWorking("contribute", phase, "td-1")
		if strings.Contains(got, "pick") {
			t.Errorf("phase %q must not advise picking up a task: %q", phase, got)
		}
		if !strings.Contains(got, "td-1") {
			t.Errorf("phase %q should name the held task: %q", phase, got)
		}
	}
	// Genuinely idle: picking up a task IS the next step.
	if got := ReplyNotWorking("submit", "idle", ""); !strings.Contains(got, "no task") {
		t.Errorf("idle reply should say there's no task: %q", got)
	}
}

// TestAgentAdviceNeverPromisesGit: an agent's /workspace is a linked worktree whose .git
// points at an unmounted host path, so EVERY git command fails there — deliberately; the
// hub is the gatekeeper for git. Advice naming BARE git is therefore a dead end. `sindri git`
// is the sanctioned path (the hub runs it), so it is stripped before the check rather than
// banned. The coauthor brief is the exception: its /workspace is the user's real checkout.
func TestAgentAdviceNeverPromisesGit(t *testing.T) {
	sandboxed := []string{
		ReplyResolveDirty("working"),
		ReplyResolveDirty("submitted"),
		ReplyResolveDirty("resolving"),
		ReplyNotWorking("contribute", "submitted", "td-1"),
		MsgReview("pr-td-1", "do the thing", "td-1", "main", "", true),
		MsgReview("pr-td-1", "do the thing", "td-1", "main", "", false),
		SystemPrompt("eitri", "worker", "", ""),
		SystemPrompt("dvalin", "reviewer", "", ""),
		DirWorking("td-1"),
		DirContainerClaimed("td-EPIC", "a feature", "td-1", "a subtask"),
	}
	for _, s := range sandboxed {
		bare := strings.ReplaceAll(s, "sindri git", "«hub-run»")
		for _, bad := range []string{"`git ", "git commit", "git diff", "git stash"} {
			if strings.Contains(bare, bad) {
				t.Errorf("advice tells a sandboxed agent to run %q: %q", bad, s)
			}
		}
	}
	// The coauthor works in the user's own checkout, so git is legitimately available.
	if !strings.Contains(SystemPrompt("brokk", "coauthor", "", ""), "git") {
		t.Error("the coauthor brief should still offer git — its /workspace is the real checkout")
	}
}

// TestAgentAdviceNeverAsksForACommit: an agent contributes or submits; the hub does the
// committing. Advice that says "commit" names an action the agent has no verb for — the
// worker directive used to require it ("when your change is committed, run submit") four
// lines under "do NOT run git", which is the same dead end in different words.
func TestAgentAdviceNeverAsksForACommit(t *testing.T) {
	for _, s := range []string{
		DirWorking("td-1"),
		DirContainerClaimed("td-EPIC", "a feature", "td-1", "a subtask"),
		ReplyResolveDirty("working"),
		ReplyResolveDirty("submitted"),
		SystemPrompt("eitri", "worker", "", ""),
		GitHelp,
	} {
		for _, bad := range []string{"commit", "Commit", "uncommitted", "Uncommitted"} {
			if strings.Contains(s, bad) {
				t.Errorf("advice puts %q on the agent — it contributes or submits, the hub commits: %q", bad, s)
			}
		}
	}
}

// TestResolveDirtyAdviceIsRunnable: the dirty-worktree reply must not point at a verb the
// hub will refuse in that phase, or it just relocates the dead end. contribute/submit
// exist only in phase "working"; under review the branch must not change at all.
func TestResolveDirtyAdviceIsRunnable(t *testing.T) {
	// Only from "working" may it name the landing verbs.
	working := ReplyResolveDirty("working")
	if !strings.Contains(working, "sindri contribute") || !strings.Contains(working, "sindri submit") {
		t.Errorf("working advice should offer contribute and submit: %q", working)
	}
	for _, phase := range []string{"submitted", "resolving", "idle"} {
		got := ReplyResolveDirty(phase)
		for _, verb := range []string{"`sindri contribute", "`sindri submit"} {
			if strings.Contains(got, verb) {
				t.Errorf("phase %q can't run %q: %q", phase, verb, got)
			}
		}
	}
	// Every phase names a runnable next step, and never the verb that never was.
	for _, phase := range []string{"working", "submitted", "resolving"} {
		got := ReplyResolveDirty(phase)
		if strings.Contains(got, "`sindri commit`") {
			t.Errorf("phase %q must not advertise a sindri commit verb: %q", phase, got)
		}
		if !strings.Contains(got, "`sindri") {
			t.Errorf("phase %q should name a runnable next step: %q", phase, got)
		}
	}
}
