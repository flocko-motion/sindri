// package: arch / wording_test
// type:    logic (one phrase, said the same way everywhere)
// job:     hold the wording rules that span PACKAGES — the help a verb prints, the standing system
// prompt, and the directive an agent is handed all have to agree, and they live in three trees.
// limits:  the agreement. Each package's own wording is tested beside it (-> hub/prompts).
package arch

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/agent/verbs"
	taskflow "github.com/flo-at/sindri/internal/hub/flow/task"
	"github.com/flo-at/sindri/internal/hub/prompts"
)

// TestParentIsStatedAsTheDefaultShape (sd-ac9800): a planner that reads `--parent` as an option for
// unusual cases keeps filing flat tasks — every surface a planner reads about create-task, at the
// standing prompt and at the moment work is handed over alike, must state hanging related work
// under a container FIRST as the default shape, not a special case.
func TestParentIsStatedAsTheDefaultShape(t *testing.T) {
	// Flattened so a source line wrapped mid-phrase (this prose is hard-wrapped for terminal
	// display) can't make a substring check miss text that reads fine to whoever receives it.
	flatten := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	for _, s := range []string{
		taskflow.CreateTaskUsage,
		taskflow.CreateTaskHelp,
		prompts.SystemPrompt("galar", "planner", "", "ARCHITECTURE.md"),
		prompts.DirPlanning,
		prompts.MsgPlanAssignment("build the thing", "", "", ""),
	} {
		flat := strings.ToLower(flatten(s))
		if !strings.Contains(flat, "the container first") {
			t.Errorf("missing container-first guidance: %q", s)
		}
		if !strings.Contains(flat, "default") || !strings.Contains(flat, "not a special case") {
			t.Errorf("missing an explicit default/not-a-special-case statement: %q", s)
		}
	}
}

// TestAgentAdviceNeverAsksForACommit: an agent contributes or submits; the hub does the
// committing. Advice that says "commit" names an action the agent has no verb for — the
// worker directive used to require it ("when your change is committed, run submit") four
// lines under "do NOT run git", which is the same dead end in different words.
func TestAgentAdviceNeverAsksForACommit(t *testing.T) {
	for _, s := range []string{
		prompts.DirWorking("td-1", 1.5, 2.0),
		prompts.DirContainerClaimed("td-EPIC", "a feature", "td-1", "a subtask"),
		prompts.DirContainerWorking("td-EPIC", "td-1", 1.5, 2.0),
		prompts.ReplyResolveDirty("working", false),
		prompts.ReplyResolveDirty("working", true),
		prompts.ReplyResolveDirty("submitted", false),
		prompts.SystemPrompt("eitri", "worker", "", ""),
		verbs.GitHelp,
		// The gate now records the workspace itself, so its replies are exactly where "just commit it"
		// would creep back in — they are the ones that know a commit happened.
		prompts.ReplyLintQueued("run-1", "abc1234", 2),
		prompts.ReplyGateReused("run-1", "abc1234"),
		prompts.MsgLintPassed("run-1"),
	} {
		for _, bad := range []string{"commit", "Commit", "uncommitted", "Uncommitted"} {
			if strings.Contains(s, bad) {
				t.Errorf("advice puts %q on the agent — it contributes or submits, the hub commits: %q", bad, s)
			}
		}
	}
}
