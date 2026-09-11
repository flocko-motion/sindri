package fleet

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	flowpr "github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// interviewing is the fixture every test here stands on: a worker inside a real worktree, holding
// work, with its own machine on demand. A real tree because the interview's whole subject is
// whether the tree it opened on is still there.
func interviewing(t *testing.T) (*Engine, *store.ProjectStore, *stubDeps, string, registry.Caller) {
	t.Helper()
	const agent, task = "bombur", "sd-1"
	root, _ := flowtest.WorkRepo(t, agent, task)
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: task, Title: "fix the flaky retry", Type: "bug", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	flowtest.Place(t, ps, store.AgentState{Agent: agent, Task: task, Branch: task, Phase: "working"})
	wt := filepath.Join(root, ".worktrees", agent)
	if err := os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("patched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := &stubDeps{Root: root}
	e := newEngine(t, st, d)
	t.Cleanup(e.Close)
	return e, ps, d, wt, registry.Caller{Project: "repo", Agent: agent, Role: "worker"}
}

// until waits for what the interview does on its own goroutine. The interview WAITS on the agent
// (-> machine.Action.Awaits), so it is the one action a test cannot drive to completion by looking:
// it is started by a look and finished by what the agent does next.
func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// stands is where the machine has the worker right now.
func stands(t *testing.T, ps *store.ProjectStore, agent string) string {
	t.Helper()
	st, err := ps.GetState(agent)
	if err != nil {
		t.Fatal(err)
	}
	return st.Phase
}

// asked drives the `submit` that asks for one, and waits until the author is standing in the
// interview with the first question in its pane.
func asked(t *testing.T, e *Engine, ps *store.ProjectStore, d *stubDeps, c registry.Caller, summary string) {
	t.Helper()
	var out bytes.Buffer
	if code, err := e.prAct().CmdSubmit(c, []string{summary}, &out); err != nil || code != 0 {
		t.Fatalf("CmdSubmit: code=%d err=%v out=%s", code, err, out.String())
	}
	e.Look(c.Project, c.Agent)
	until(t, "the first question", func() bool {
		return stands(t, ps, c.Agent) == "worker/interviewing" && said(d, "question 1 of 2") == 1
	})
}

// said counts how many delivered messages carry want.
func said(d *stubDeps, want string) int {
	n := 0
	for _, m := range d.Said() {
		if strings.Contains(m, want) {
			n++
		}
	}
	return n
}

// answer records one answer the way the agent does — the same verb it used to ask.
func answer(t *testing.T, e *Engine, c registry.Caller) {
	t.Helper()
	const text = "I swept every call site this touches, and each one is covered by a test that fails without it."
	var out bytes.Buffer
	if code, err := e.prAct().CmdSubmit(c, []string{text}, &out); err != nil || code != 0 {
		t.Fatalf("answering: code=%d err=%v out=%s", code, err, out.String())
	}
}

// headOf is the worktree's current commit, so a test can say whether anything was committed.
func headOf(t *testing.T, dir string) string {
	t.Helper()
	sha, err := git.Head(dir)
	if err != nil {
		t.Fatalf("head of %s: %v", dir, err)
	}
	return sha
}

// TestAskingToSubmitOpensTheInterviewAndCommitsNothing: the questions come BEFORE the commit now,
// because the answers describe the tree as it stands and committing it first made the answers
// describe something the author could no longer be held to.
func TestAskingToSubmitOpensTheInterviewAndCommitsNothing(t *testing.T) {
	e, ps, d, wt, c := interviewing(t)
	before := headOf(t, wt)

	asked(t, e, ps, d, c, "retry with backoff")

	if got := headOf(t, wt); got != before {
		t.Errorf("the tree was committed before its author had answered for it: %s -> %s", before, got)
	}
	if _, _, ok := e.runAct().NextQueuedRun(); ok {
		t.Error("a gate was queued before the questions were answered")
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Error("a PR exists before the questions were answered")
	}
}

// TestEveryQuestionAnsweredTakesTheSubmit is the interview's expected exit: the answers are in, so
// the author leaves for the state that commits and gates, with nothing further asked of it.
func TestEveryQuestionAnsweredTakesTheSubmit(t *testing.T) {
	e, ps, d, _, c := interviewing(t)
	asked(t, e, ps, d, c, "retry with backoff")

	answer(t, e, c)
	until(t, "the second question", func() bool { return said(d, "question 2 of 2") == 1 })
	answer(t, e, c)
	until(t, "the submit to be taken", func() bool { return stands(t, ps, c.Agent) == "worker/submitting" })

	e.Look(c.Project, c.Agent) // the state the interview left it in acts on the next pass
	if got := stands(t, ps, c.Agent); got != "worker/gating" {
		t.Fatalf("an answered submit should reach the gate, it stands in %q", got)
	}
	if _, _, ok := e.runAct().NextQueuedRun(); !ok {
		t.Error("the submit was taken without a gate being queued")
	}
	if _, ok, _ := ps.SubmitAsked(c.Agent); ok {
		t.Error("the request stands after being carried out, so the author would be asked again for ever")
	}
}

// TestEditingTheCodeAbandonsTheInterview: the answers describe the tree they were asked about, so an
// author that edits it mid-interview has answered for a tree that will not be the one going up. It
// goes back to the work rather than submitting something nobody answered for.
func TestEditingTheCodeAbandonsTheInterview(t *testing.T) {
	e, ps, d, wt, c := interviewing(t)
	asked(t, e, ps, d, c, "retry with backoff")

	if err := os.WriteFile(filepath.Join(wt, "more.txt"), []byte("second thoughts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	answer(t, e, c)

	until(t, "the author to be put back on the work", func() bool {
		return stands(t, ps, c.Agent) == "worker/working"
	})
	if _, ok, _ := ps.SubmitAsked(c.Agent); ok {
		t.Error("an abandoned submit left its request standing")
	}
	if _, rows, _ := ps.OpenSubmitAnswers(c.Agent); len(rows) != 0 {
		t.Error("an abandoned interview is still open, so the next submit would inherit its answers")
	}
	if _, _, ok := e.runAct().NextQueuedRun(); ok {
		t.Error("a submit nobody answered for reached the gate")
	}
}

// TestADroppedQuestionIsPutAgain: a push that never landed leaves the author at an empty prompt with
// a question standing against it, which is a submit that waits for ever. An author AT A PROMPT has
// ended its turn, so the question goes again.
func TestADroppedQuestionIsPutAgain(t *testing.T) {
	e, ps, d, _, c := interviewing(t)
	asked(t, e, ps, d, c, "retry with backoff")

	// The reading is placed after the question: no observer sweeps in a test, and a reading older
	// than the question proves nothing about whether the author saw it.
	d.ObservedAt(time.Now().Add(2 * time.Minute))
	until(t, "the question to be put again", func() bool { return said(d, "question 1 of 2") > 1 })
	if got := stands(t, ps, c.Agent); got != "worker/interviewing" {
		t.Errorf("re-posing must not move the author, it stands in %q", got)
	}
}

// TestAnAuthorResearchingIsLeftAlone is the other half, and the reason the re-posing keys on the
// prompt rather than on a dwell: going to the code is the behaviour these questions exist to
// provoke, so the agent doing it is exactly the one a timer would nag.
func TestAnAuthorResearchingIsLeftAlone(t *testing.T) {
	e, ps, d, _, c := interviewing(t)
	asked(t, e, ps, d, c, "retry with backoff")

	d.Turning(c.Agent, true) // its session reports a turn in progress
	d.ObservedAt(time.Now().Add(2 * time.Minute))
	time.Sleep(3 * interviewBeat)
	if n := said(d, "question 1 of 2"); n != 1 {
		t.Errorf("an author mid-turn was asked again %d times", n-1)
	}
	if got := stands(t, ps, c.Agent); got != "worker/interviewing" {
		t.Errorf("an author taking its time should still be interviewing, it stands in %q", got)
	}
}

// TestAnAbandonedInterviewIsVisible: an author that simply stops answering must READ as what it is.
// Held inside "working" this was an agent the board showed as busy on a task it had stopped, and the
// dwell that says how long is the state's own.
func TestAnAbandonedInterviewIsVisible(t *testing.T) {
	e, ps, d, _, c := interviewing(t)
	asked(t, e, ps, d, c, "retry with backoff")

	sit, err := e.Sit.Of(c.Project, c.Agent)
	if err != nil {
		t.Fatal(err)
	}
	if sit.Phase != "worker/interviewing" {
		t.Errorf("the board shows %q rather than the interview it is standing in", sit.Phase)
	}
	if st, _ := ps.GetState(c.Agent); st.PhaseSince == "" {
		t.Error("the state carries no stamp, so nothing can say how long it has stood")
	}
	// And the question it owes is what it is told when it asks, since the one in its pane has
	// scrolled away by the time it wonders.
	words, err := e.stands(c.Project, c.Agent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(words, flowpr.QGeneralise) {
		t.Errorf("an author asking where it stands should hear the question back:\n%s", words)
	}
}
