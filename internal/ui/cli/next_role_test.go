package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// refuse runs a built command with args and returns what it refused with. It never reaches the
// hub: every case here is decided in front of the user, which is the point — a contradiction is
// caught where it was typed rather than carried to the server.
func refuse(cmd *cobra.Command, args ...string) error {
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard) // cobra's usage dump is not what these cases are about
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

// TestAgentAndRoleCannotBeCombined: an agent already HAS a role, so passing both states two things
// that can contradict — and picking a winner would hide the contradiction rather than show it.
func TestAgentAndRoleCannotBeCombined(t *testing.T) {
	err := refuse(taskNextCmd(), "--agent", "bombur", "--role", "worker")
	if err == nil {
		t.Fatal("--agent with --role must be refused")
	}
	if !strings.Contains(err.Error(), "agent") || !strings.Contains(err.Error(), "role") {
		t.Errorf("the refusal must name both flags, got %q", err)
	}
}

// TestAReviewerIsSentToThePRCommand: the answer for a reviewer is a PR, and a command whose noun is
// `task` has no business claiming otherwise — so it names the one that does.
func TestAReviewerIsSentToThePRCommand(t *testing.T) {
	err := refuse(taskNextCmd(), "--role", "reviewer")
	if err == nil {
		t.Fatal("`task next --role reviewer` must not answer with PRs under a task noun")
	}
	if !strings.Contains(err.Error(), "pr next") {
		t.Errorf("the refusal must name the command that answers, got %q", err)
	}
}

// TestPRNextTakesOnlyAReviewer: the other roles are served from the backlog, and pointing at the
// command that answers beats "unknown role".
func TestPRNextTakesOnlyAReviewer(t *testing.T) {
	err := refuse(prNextCmd(), "--role", "worker")
	if err == nil {
		t.Fatal("`pr next --role worker` must be refused — a worker is served tasks")
	}
	if !strings.Contains(err.Error(), "task next") {
		t.Errorf("the refusal must name the command that answers, got %q", err)
	}
}

// TestTheNounHoldsForANamedAgentToo is the half the flags alone did not cover. --role is checked
// before the call, but a named agent's role is the hub's to say, so the check has to happen after
// it — otherwise `task next --agent <a reviewer>` prints a PR table under a task noun, which is
// precisely what the flag path refuses.
func TestTheNounHoldsForANamedAgentToo(t *testing.T) {
	err := wrongNounRefusal("reviewer", "dvalin")
	if err == nil || !strings.Contains(err.Error(), "pr next --agent dvalin") {
		t.Errorf("a reviewer asked about under the task noun must be sent to `pr next`, got %v", err)
	}
	for _, role := range []string{"worker", "planner", "coauthor"} {
		err := wrongNounRefusal(role, "nori")
		if err == nil || !strings.Contains(err.Error(), "task next --agent nori") {
			t.Errorf("a %s asked about under the PR noun must be sent to `task next`, got %v", role, err)
		}
		if !strings.Contains(err.Error(), role) {
			t.Errorf("the refusal should name what the agent actually is, got %v", err)
		}
	}
}

// TestOnlyAReviewerIsAboutPRs pins the distinction both commands turn on, so a new role added
// later cannot silently join the PR side by omission.
func TestOnlyAReviewerIsAboutPRs(t *testing.T) {
	if !nextIsAboutPRs("reviewer") {
		t.Error("a reviewer is served PRs")
	}
	for _, role := range []string{"worker", "planner", "coauthor", ""} {
		if nextIsAboutPRs(role) {
			t.Errorf("%q is not served PRs", role)
		}
	}
}
