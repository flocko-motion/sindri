// package: arch / wording_test
// type:    logic (one phrase, said the same way everywhere)
// job:     hold the wording rules that span PACKAGES — the help a verb prints, the standing system
// prompt, and the directive an agent is handed all have to agree, and they live in three trees.
// limits:  the agreement. Each package's own wording is tested beside it (-> hub/prompts).
package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/agent/workspace"
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
		prompts.DirContainerWorking("td-EPIC", "td-1", 1.5, 2.0),
		prompts.ReplyResolveDirty("working", false),
		prompts.ReplyResolveDirty("working", true),
		prompts.ReplyResolveDirty("submitted", false),
		prompts.SystemPrompt("eitri", "worker", "", ""),
		workspace.GitHelp,
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

// asksForItsDirective matches the bare `sindri` ask — the no-arg call that answers where an agent
// stands. A pointer at a NAMED verb (`sindri submit`, `sindri task list`) carries no match, because
// naming a verb tells the agent what to do rather than sending it to find out.
var asksForItsDirective = regexp.MustCompile("[Rr]un `sindri`")

// directiveAskAllowed is the one place the ask belongs: mail is read when the AGENT chooses, so
// telling it that mail is waiting and leaving the timing to it is the whole point of the message.
var directiveAskAllowed = map[string]bool{
	filepath.Join("internal", "hub", "messaging", "mail", "announce.go"): true,
}

// TestNothingTellsAnAgentToAskForItsDirective: the hub dispatches. It selects, prepares and
// instructs, so it is holding the answer at the moment it speaks — and a reply that sends the agent
// to fetch it costs a call and a turn for something already in hand. Worse, the fetch can be lost:
// a pane interrupted between the two leaves the agent waiting on a hub that believes it dispatched.
// Where there is something to do, the hub says it; where there is not, it says nothing.
func TestNothingTellsAnAgentToAskForItsDirective(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if vocabSkipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		if directiveAskAllowed[rel] {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(b), "\n") {
			if asksForItsDirective.MatchString(line) {
				t.Errorf("%s:%d tells an agent to ask for what the hub is holding: %q",
					rel, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
}
