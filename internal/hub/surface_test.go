package hub

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// surfaceFor seeds a worker in the given phase and returns the verbs it can RUN right now. The
// listing also carries the ones it cannot, each with its reason (-> runnableFor's counterpart), so
// presence in the list is no longer the same question as being able to run it.
func surfaceFor(t *testing.T, phase string) (*Hub, []string) {
	t.Helper()
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	task := "td-1"
	if phase == "idle" {
		task = ""
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: task, Branch: task, Phase: phase}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	cmds, err := h.AgentCommands(testProject, "eitri")
	if err != nil {
		t.Fatalf("agent commands: %v", err)
	}
	var names []string
	for _, c := range cmds {
		if c.Unavailable == "" {
			names = append(names, c.Name)
		}
	}
	return h, names
}

// blockedReason returns why a worker in phase cannot run verb, "" when it can.
func blockedReason(t *testing.T, phase, verb string) string {
	t.Helper()
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	task := "td-1"
	if phase == "idle" {
		task = ""
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: task, Branch: task, Phase: phase}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	cmds, err := h.AgentCommands(testProject, "eitri")
	if err != nil {
		t.Fatalf("agent commands: %v", err)
	}
	for _, c := range cmds {
		if c.Name == verb {
			return c.Unavailable
		}
	}
	t.Fatalf("%q is a worker verb and must be listed in any phase, blocked or not", verb)
	return ""
}

func has(names []string, v string) bool {
	for _, n := range names {
		if n == v {
			return true
		}
	}
	return false
}

// TestWorkVerbsOnlyOfferedWhileWorking is the invariant that broke in the field: submit
// and contribute both guard on `st.Phase != "working"`, so the surface must not offer
// them in any other phase. A worker whose PR was under review WAS offered them, ran
// one, and got "run `sindri` to pick up a task first" — a hub telling an agent holding
// a task to go get a task.
func TestWorkVerbsOnlyOfferedWhileWorking(t *testing.T) {
	for _, tc := range []struct {
		phase string
		want  bool
	}{
		{"working", true},
		{"submitted", false},
		{"resolving", false},
		{"idle", false},
	} {
		_, names := surfaceFor(t, tc.phase)
		for _, verb := range []string{"submit", "contribute"} {
			if got := has(names, verb); got != tc.want {
				t.Errorf("phase %q: %s runnable=%v, want %v (runnable: %v)", tc.phase, verb, got, tc.want, names)
			}
			// Held back, it is still LISTED and says why. Dropping it taught an agent mid-review
			// that submit did not exist, so it stopped rather than asking.
			if !tc.want && blockedReason(t, tc.phase, verb) == "" {
				t.Errorf("phase %q: %s is held back but the listing gives no reason", tc.phase, verb)
			}
		}
	}
}

// TestResolveAlwaysOffered: checking the branch still merges is harmless in every
// phase, and the "wait for your verdict" directive explicitly points at it.
func TestResolveAlwaysOffered(t *testing.T) {
	for _, phase := range []string{"working", "submitted", "resolving", "idle"} {
		if _, names := surfaceFor(t, phase); !has(names, "resolve") {
			t.Errorf("phase %q should offer resolve, got %v", phase, names)
		}
	}
}

// TestEveryOfferedVerbRuns is the general form of the same bug: whatever the hub
// advertises must not immediately refuse as inapplicable. Read-only verbs are exercised
// for real; the rest only have to avoid the "unknown or unavailable" reply, which is
// what an agent hit when it guessed at `commit`.
func TestEveryOfferedVerbRuns(t *testing.T) {
	for _, phase := range []string{"working", "submitted", "resolving", "idle"} {
		h, names := surfaceFor(t, phase)
		for _, verb := range names {
			if verb != "status" && verb != "prs" {
				continue // the rest need a repo/worktree; surface membership is the point here
			}
			var out bytes.Buffer
			if _, err := h.AgentExec(testProject, "eitri", []string{verb}, &out); err != nil {
				t.Errorf("phase %q: offered verb %q errored: %v", phase, verb, err)
			}
			if strings.Contains(out.String(), "unknown or unavailable") {
				t.Errorf("phase %q: offered verb %q was rejected as unavailable", phase, verb)
			}
		}
	}
}

// TestUnknownVerbListsWhatIsAvailable: a bare "unknown command" made the agent guess,
// and each guess cost a turn. The reply has to name the real surface.
func TestUnknownVerbListsWhatIsAvailable(t *testing.T) {
	h, names := surfaceFor(t, "submitted")
	var out bytes.Buffer
	code, err := h.AgentExec(testProject, "eitri", []string{"commit"}, &out)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if code != 127 {
		t.Errorf("unknown verb exit = %d, want 127", code)
	}
	got := out.String()
	if !strings.Contains(got, "unknown or unavailable command: commit") {
		t.Errorf("should name the bad verb, got:\n%s", got)
	}
	for _, verb := range names {
		if !strings.Contains(got, verb) {
			t.Errorf("should list available verb %q, got:\n%s", verb, got)
		}
	}
}
