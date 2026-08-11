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

// TestPlannerBriefOffersTaskOnlyPath: create-task and --parent hierarchies already worked, but
// the durable brief framed openspec as the only way a plan concludes. It must now say a plan can
// end in nothing but approved backlog tasks — no spec, no PR — and give the hierarchy vocabulary
// (--parent, --type epic) hierarchies need. Other roles never see planner-only guidance.
func TestPlannerBriefOffersTaskOnlyPath(t *testing.T) {
	p := SystemPrompt("galar", "planner", "", "ARCHITECTURE.md")
	for _, want := range []string{
		"a complete outcome",
		"--parent",
		"--type epic",
		"sindri state idle",
		"no spec, no PR required",
		"for work that needs one",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("the planner brief must offer the task-only path, missing %q:\n%s", want, p)
		}
	}
	for _, role := range []string{"worker", "reviewer", "coauthor"} {
		if p := SystemPrompt("x", role, "", "ARCHITECTURE.md"); strings.Contains(p, "create-task") {
			t.Errorf("%s should not see the planner's create-task guidance:\n%s", role, p)
		}
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
		ReplyResolveDirty("working", false),
		ReplyResolveDirty("working", true),
		ReplyResolveDirty("submitted", false),
		ReplyResolveDirty("resolving", false),
		ReplyNotWorking("contribute", "submitted", "td-1"),
		MsgReview("pr-td-1", "do the thing", "td-1", "main", "", true),
		MsgReview("pr-td-1", "do the thing", "td-1", "main", "", false),
		SystemPrompt("eitri", "worker", "", ""),
		SystemPrompt("dvalin", "reviewer", "", ""),
		DirWorking("td-1"),
		DirContainerClaimed("td-EPIC", "a feature", "td-1", "a subtask"),
		DirContainerWorking("td-EPIC", "td-1"),
		DirContainerRejected("td-EPIC", "td-1", "not yet"),
		MsgMilestoneRejected("td-EPIC", "user", "not yet"),
		ReplyCheckpointed("td-1", "td-2", "the next subtask"),
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
		DirContainerWorking("td-EPIC", "td-1"),
		ReplyResolveDirty("working", false),
		ReplyResolveDirty("working", true),
		ReplyResolveDirty("submitted", false),
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
	working := ReplyResolveDirty("working", false)
	if !strings.Contains(working, "sindri contribute") || !strings.Contains(working, "sindri submit") {
		t.Errorf("working advice should offer contribute and submit: %q", working)
	}
	for _, phase := range []string{"submitted", "resolving", "idle"} {
		got := ReplyResolveDirty(phase, false)
		for _, verb := range []string{"`sindri contribute", "`sindri submit"} {
			if strings.Contains(got, verb) {
				t.Errorf("phase %q can't run %q: %q", phase, verb, got)
			}
		}
	}
	// Every phase names a runnable next step, and never the verb that never was.
	for _, phase := range []string{"working", "submitted", "resolving"} {
		got := ReplyResolveDirty(phase, false)
		if strings.Contains(got, "`sindri commit`") {
			t.Errorf("phase %q must not advertise a sindri commit verb: %q", phase, got)
		}
		if !strings.Contains(got, "`sindri") {
			t.Errorf("phase %q should name a runnable next step: %q", phase, got)
		}
	}
	// A feature worker holds checkpoint in place of contribute and submit, so the same reply must
	// swap the verb rather than send it to two it cannot run.
	inContainer := ReplyResolveDirty("working", true)
	if !strings.Contains(inContainer, "`sindri checkpoint") {
		t.Errorf("a feature worker's advice should offer checkpoint: %q", inContainer)
	}
	for _, verb := range []string{"`sindri contribute", "`sindri submit"} {
		if strings.Contains(inContainer, verb) {
			t.Errorf("a feature worker can't run %q: %q", verb, inContainer)
		}
	}
}

// TestAssignedWorkSaysItStartsNow: a hand-off that assigns work has to say the work is the agent's
// to start, and the ones that genuinely block have to name the wait. A worker read the checkpoint
// hand-off as a report, announced its next subtask, and then waited for a go-ahead from the user
// that the workflow never sends — the reply named the subtask but never said to begin it.
func TestAssignedWorkSaysItStartsNow(t *testing.T) {
	for _, s := range []string{
		ReplyCheckpointed("td-1", "td-2", "the next subtask"),
		DirContainerWorking("td-EPIC", "td-1"),
	} {
		if !strings.Contains(s, "starts now") && !strings.Contains(s, "Implement it") {
			t.Errorf("a hand-off that assigns work must say to start it: %q", s)
		}
	}
	// Clearing a feature's last subtask is a hand-off too — to the submit — so it must read as an
	// instruction rather than as leave to stop.
	for _, s := range []string{
		ReplyCheckpointedLast("td-2", "td-EPIC"),
		DirContainerDone("td-EPIC"),
	} {
		if !strings.Contains(s, "`sindri submit") {
			t.Errorf("a finished feature must be told to put itself up: %q", s)
		}
		if strings.Contains(s, "Wait") {
			t.Errorf("nothing waits once a feature is built: %q", s)
		}
	}
	// The one hand-off that really does block still names the wait.
	if s := DirSubmitted; !strings.Contains(s, "Wait") {
		t.Errorf("a PR under review must name the wait: %q", s)
	}
	// The briefs carry the rule the individual hand-offs then rely on, since they are read before any
	// directive arrives: every managed role learns that a wait is always named as one, and a worker
	// that its assigned work needs no further go-ahead.
	for _, role := range []string{"worker", "reviewer", "planner"} {
		brief := SystemPrompt("dvalin", role, "", "")
		if !strings.Contains(brief, "WAITING IS ALWAYS NAMED") {
			t.Errorf("the %s brief should say a wait is always named as one:\n%s", role, brief)
		}
	}
	if brief := SystemPrompt("dvalin", "worker", "", ""); !strings.Contains(brief, "already authorised") {
		t.Errorf("the worker brief should state that assigned work needs no further go-ahead:\n%s", brief)
	}
	// The planner is the one role with a real standing wait, and it must still be named as one so the
	// rule above doesn't read as licence to start writing.
	if planner := SystemPrompt("galar", "planner", "", ""); !strings.Contains(planner, GoToken) {
		t.Errorf("the planner brief must still name its own wait (%s):\n%s", GoToken, planner)
	}
}

// TestMidFeatureAdviceNamesCheckpoint is the rule dvalin's report exposed, in the form it takes now
// that a finished feature submits itself: every instruction reaching a worker with subtasks still to
// do must name only verbs open to it there. While the branch is incomplete that is checkpoint, and
// contribute for a branch worth sharing early; submit is held back until the last subtask.
func TestMidFeatureAdviceNamesCheckpoint(t *testing.T) {
	for _, s := range []string{
		DirContainerClaimed("td-EPIC", "a feature", "td-1", "a subtask"),
		DirContainerWorking("td-EPIC", "td-1"),
		ReplyCheckpointed("td-1", "td-2", "the next subtask"),
		ReplySubtasksRemain("td-EPIC", "td-2", 3),
		ReplyResolveDirty("working", true),
	} {
		if !strings.Contains(s, "`sindri checkpoint") {
			t.Errorf("mid-feature advice must name checkpoint: %q", s)
		}
		if strings.Contains(s, "`sindri next") {
			t.Errorf("advice to a feature worker names `sindri next`, which its surface hides: %q", s)
		}
	}
	// Where a feature's advice DOES name submit, it must be about the whole branch — never something
	// to do per subtask, which is the confusion the checkpoint flow exists to prevent.
	for _, s := range []string{
		DirContainerClaimed("td-EPIC", "a feature", "td-1", "a subtask"),
		DirContainerWorking("td-EPIC", "td-1"),
	} {
		if !strings.Contains(s, "never per subtask") {
			t.Errorf("mid-feature advice must rule out a per-subtask submit: %q", s)
		}
	}
	// Once the branch is complete, submit is exactly the verb — including after a rejection, which
	// puts the worker back on the same branch to fix and resubmit.
	for _, s := range []string{
		DirContainerDone("td-EPIC"),
		ReplyCheckpointedLast("td-2", "td-EPIC"),
		DirContainerRejected("td-EPIC", "td-1", "not yet"),
		MsgMilestoneRejected("td-EPIC", "reviewer", "not yet"),
	} {
		if !strings.Contains(s, "`sindri submit") {
			t.Errorf("a complete feature must be told to submit: %q", s)
		}
	}
}
