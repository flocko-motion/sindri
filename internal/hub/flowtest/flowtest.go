// package: hub/flowtest / flowtest
// type:    assembly (one fake hub, for every subject's tests)
// job:     stand in for the hub on both seams a subject is handed — the box its agents live in and
// the facts the orchestrator owns — recording what was asked of it, so a test in flow/pr, flow/task
// or flow/agent asserts against a store and a recorder rather than against a whole Hub.
// limits:  a fake, and nothing more. Every rule stays in the subject under test; this only answers
// and remembers. Outside the flow tree on purpose — it reaches writable things by design.
package flowtest

import (
	"context"
	"errors"
	"fmt"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/world/observe"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// Hub is a no-op fleet.Deps that records the agents it interrupts/injects —
// enough to drive ScrapPR without a real hub. (First workflow Engine test harness;
// extend as more Engine methods get covered.)
type Hub struct {
	Root         string
	Alive        bool
	Interrupted  []string
	Injected     []string
	InjectedText []string                   // the message bodies too, for tests that assert what an agent was told
	CtxTokens    int                        // TestContextFull* set these to simulate a worker's session usage
	CtxWindow    int                        // 0 with ctxOK true means "measured, but the window is unknown"
	CtxOK        bool                       // false = nothing recorded yet, which the observation says as a zero window
	Comments     map[string][]store.Comment // by task id, for the views that render a thread
	Busy         map[string]bool            // agents mid-turn, so AgentIdle answers false for them
	Posted       []store.Comment            // what the workflow wrote onto a task's thread (SourceRef holds the id)
	PostFails    bool                       // AddTaskComment refuses, for the paths that must survive it
	Delivered    []mail.Delivery            // how each message was classified, in step with injected/injectedText
	DeliverErr   bool                       // Deliver refuses, for the paths that must not record an undelivered message
	Projects     []store.Project            // KnownProjects override; nil (the default) means none registered
	Model        string                     // CurrentModel's answer; "" is fine — no real model is ever ""
	// The observation's own fields (-> Observe): the session's runtime word, and how long the pane
	// has stood still.
	Runtime  string
	StillFor time.Duration
	// tierModels overrides ModelForTier's answer; nil (the default) means every tier is unknown, so
	// the retier check never fires for a test that has not opted into it.
	TierModels  map[string]string
	ModelSet    []string // "name=model" for every SetModel call, in order
	SetModelErr error
	EmptyHanded bool
	compactErr  error
	Cleared     []string // agents Clear was called for, in order
	ClearErr    error
	// Config overrides what ProjectConfig answers; the zero value (no lint.max_comment_avg set)
	// means the caller sees no override, same as an unconfigured project.
	Config    config.Config
	ConfigErr error
	// escalated records Escalate calls as "name: question", in order.
	Escalated []string
	// Looked and Woke record the re-decisions an act asked for, in order.
	Looked   []string
	Woke     []string
	started  []string // agents StartAgent was called for, in order
	startErr error
}

// ProjectRoot is the one repo these fixtures registered. It PANICS on an unset root rather than
// answering "": a relative path is joined against the package directory, so a fixture that forgot
// its root wrote a git repo into the source tree (internal/hub/flow/fleet/wt) and nothing said so.
func (d *Hub) ProjectRoot(string) string {
	if d.Root == "" {
		panic("flowtest: this fixture has no Root, so every path it hands out lands in the source tree")
	}
	return d.Root
}

// Saying is an observation of a LIVE session reporting word, for the nudge paths that used to take
// the bare string. The word crosses inside the observation now, which is the whole point of the split.
func Saying(word string) observe.Observation {
	return observe.Observation{TakenAt: time.Now(), Up: true, State: observe.ParseState(word)}
}

// SayingWhileDown is the same reading of a pod that is gone. Liveness rides the observation rather
// than being asked separately, so a caller cannot judge one reading and prod on another.
func SayingWhileDown(word string) observe.Observation {
	o := Saying(word)
	o.Up = false
	return o
}

// testGate is the passing gate every fixture gets unless it declares its own. A project MUST declare
// one (-> repo.Gate), so an unconfigured stub would refuse every submit in this package and test the
// refusal rather than the flow under examination.
const testGate = "sindri-test-gate.sh"

// ProjectConfig answers with Config, writing a gate into the root when none is declared —
// every fixture here that submits needs one, and none of them cares what it runs.
func (d *Hub) ProjectConfig(string) (config.Config, error) {
	if d.ConfigErr != nil {
		return config.Config{}, d.ConfigErr
	}
	cfg := d.Config
	if cfg.Verify == "" && d.Root != "" {
		// Written where the gate looks for it — runVerify stats the path in the worktree it is
		// checking, and every fixture here gates the root or a worktree built out of it.
		d.WriteGate(d.Root)
		cfg.Verify = "./" + testGate // a command, run through a shell, so a bare name would need PATH
	}
	return cfg, nil
}

// WriteGate materialises the stub's gate, in the root and in any worktree under it, since the
// gate runs against whichever tree the run named. Excluded from git as it is written: several tests
// assert the gate leaves a CLEAN tree, and an untracked script of our own would be the dirt.
func (d *Hub) WriteGate(root string) {
	_ = os.WriteFile(filepath.Join(root, ".git", "info", "exclude"), []byte(testGate+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, testGate), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	entries, err := os.ReadDir(filepath.Join(root, ".worktrees"))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			_ = os.WriteFile(filepath.Join(root, ".worktrees", e.Name(), testGate), []byte("#!/bin/sh\nexit 0\n"), 0o755)
		}
	}
}

// Escalate records the question, so a test can assert the hub stopped an agent rather than merely
// telling it something it could not act on.
func (d *Hub) Escalate(_, name, question string) (string, error) {
	d.Escalated = append(d.Escalated, name+": "+question)
	return "", nil
}

// ArchitectureDoc names no doc: no fixture asserts on one.
func (d *Hub) ArchitectureDoc(string) string { return "" }

// Container names a pod after its agent, which is all any assertion here reads.
func (d *Hub) Container(_, name string) string { return name }

// Notify answers as a hub would, recording the ask where a test can read it.
func (d *Hub) Notify() {}

// The mailbox's own seam, so one stub still answers for the whole hub. Push is what Say already
// records; MayWake and RepoName are the two questions announcing asks.
func (d *Hub) Push(project, name, text string) error {
	return d.Say(project, name, text, mail.PushOnly)
}

// MayWake answers as a hub would, recording the ask where a test can read it.
func (d *Hub) MayWake(string, string) bool { return true }

// Reachable answers as a hub would, recording the ask where a test can read it.
func (d *Hub) Reachable(p, name string) bool { return d.Observe(p, name).Up }

// RepoName answers as a hub would, recording the ask where a test can read it.
func (d *Hub) RepoName(project string) string { return project }

// Deliver records what was sent and HOW, so a test can assert the classification a sender chose —
// which is half of what this feature is (-> mail.Delivery). The recipient/text lists stay as they
// were, since every existing assertion about "what was injected" is about the same messages.
func (d *Hub) Deliver(_, name, text string, del mail.Delivery) error {
	if d.DeliverErr {
		return fmt.Errorf("nothing could be delivered to %s", name)
	}
	d.Injected = append(d.Injected, name)
	d.InjectedText = append(d.InjectedText, text)
	d.Delivered = append(d.Delivered, del)
	return nil
}

// Interrupt answers as a hub would, recording the ask where a test can read it.
func (d *Hub) Interrupt(_, name string) error {
	d.Interrupted = append(d.Interrupted, name)
	return nil
}

// Observe is the harness's standing look, off the same fields the fixture already sets. Probe is the
// fresh one; nothing here distinguishes them, since no test in this package turns on the difference.
func (d *Hub) Observe(_, name string) observe.Observation {
	// An agent nothing marked busy is AT A PROMPT, which is what the sweeps read; `busy` says a turn
	// is running, and an explicit runtime beats both.
	state := observe.ParseState(d.Runtime)
	if d.Runtime == "" {
		state = observe.AtPrompt
		if d.Busy[name] {
			state = observe.Working
		}
	}
	o := observe.Observation{
		TakenAt: time.Now(), Up: d.Alive, State: state, Model: d.Model,
		StillSince: time.Now().Add(-d.StillFor),
	}
	if d.CtxOK {
		// Nothing recorded yet is a zero window, which is how the observation says "unreadable" —
		// there is no separate ok flag to carry any more.
		o.Fill, o.Window = d.CtxTokens, d.CtxWindow
	}
	return o
}

// Probe answers as a hub would, recording the ask where a test can read it.
func (d *Hub) Probe(project, name string) observe.Observation { return d.Observe(project, name) }

// Say answers as a hub would, recording the ask where a test can read it.
func (d *Hub) Say(project, name, text string, del mail.Delivery) error {
	return d.Deliver(project, name, text, del)
}

// Start records who was woken, so a test can assert work arriving for an empty pool brings a
// reviewer back rather than waiting for one that never comes.
func (d *Hub) Start(_, name string) error {
	d.started = append(d.started, name)
	return d.startErr
}

// TaskComments answers as a hub would, recording the ask where a test can read it.
func (d *Hub) TaskComments(_, id string) []store.Comment { return d.Comments[id] }

// AddTaskComment answers as a hub would, recording the ask where a test can read it.
func (d *Hub) AddTaskComment(_, id, author, body string) error {
	if d.PostFails {
		return errors.New("the thread is unreachable")
	}
	d.Posted = append(d.Posted, store.Comment{SourceRef: id, Author: author, Body: body})
	return nil
}

// KnownProjects answers as a hub would, recording the ask where a test can read it.
func (d *Hub) KnownProjects() []store.Project { return d.Projects }

// ContextUsage answers as a hub would, recording the ask where a test can read it.
func (d *Hub) ContextUsage(_, _ string) (int, int, string, bool) {
	return d.CtxTokens, d.CtxWindow, "", d.CtxOK
}

// CurrentModel answers as a hub would, recording the ask where a test can read it.
func (d *Hub) CurrentModel(_, _ string) string { return d.Model }

// ModelForTier answers as a hub would, recording the ask where a test can read it.
func (d *Hub) ModelForTier(tier string) (string, bool) {
	m, ok := d.TierModels[tier]
	return m, ok
}

// ModelMatches mirrors the real adapter's own substring tolerance (a detected model id may carry
// more than the plain tier id names, e.g. a dated snapshot suffix) rather than a bare equality —
// exact-string test cases pass either way, since a string always contains itself.
func (d *Hub) ModelMatches(want, detected string) bool {
	return strings.Contains(detected, want)
}

// SetModel answers as a hub would, recording the ask where a test can read it.
func (d *Hub) SetModel(_ context.Context, _, name, model string) error {
	d.ModelSet = append(d.ModelSet, name+"="+model)
	return d.SetModelErr
}

// HoldsNothing answers as a hub would, recording the ask where a test can read it.
func (d *Hub) HoldsNothing(_, _, _ string) (bool, error) { return d.EmptyHanded, nil }

// Clear answers as a hub would, recording the ask where a test can read it.
func (d *Hub) Clear(_ context.Context, _, name string) error {
	d.Cleared = append(d.Cleared, name)
	return d.ClearErr
}

// TestProject is the one repo these fixtures register, named as every subject's tests name theirs.
const TestProject = "repo"

// Core is a subject's handles over a real store and this fake hub, with the project registered and
// a workspace on disk. The store is real on purpose: what a subject GUARANTEES is what survives a
// write, and a fake one would assert nothing about that.
func Core(t *testing.T, d *Hub) (*core.Core, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if d.Root == "" {
		d.Root = t.TempDir()
	}
	if err := st.RegisterProject(TestProject, d.Root); err != nil {
		t.Fatal(err)
	}
	c := &core.Core{
		Store: st, Deps: d, Harness: d,
		Sit:      situation.NewGatherer(st, d),
		Lifetime: t.Context(),
		Mail:     mail.New(st, d),
		Flow:     d,
		Pre:      core.Preflight{Seen: map[string]string{}},
	}
	return c, st.For(TestProject)
}

// Over is Core for a caller that already opened a store and seeded it — the same handles, without
// registering a project this one has already made.
func Over(st *store.Store, d *Hub) *core.Core {
	return &core.Core{
		Store: st, Deps: d, Harness: d,
		Sit:      situation.NewGatherer(st, d),
		Lifetime: context.Background(),
		Mail:     mail.New(st, d),
		Flow:     d,
		Pre:      core.Preflight{Seen: map[string]string{}},
	}
}

// The re-decision seam (-> core.Flows). An act that changed the world tells the machines to look
// again; here that is recorded rather than run, so a test can assert what a write UNSETTLED without
// standing up a machine to watch it happen.
func (d *Hub) Look(project, agent string) { d.Looked = append(d.Looked, project+"/"+agent) }

// LookPRs answers as a hub would, recording the ask where a test can read it.
func (d *Hub) LookPRs(project string) { d.Looked = append(d.Looked, project+"/prs") }

// LookTask answers as a hub would, recording the ask where a test can read it.
func (d *Hub) LookTask(project, id string) { d.Looked = append(d.Looked, project+"/"+id) }

// LookTasks answers as a hub would, recording the ask where a test can read it.
func (d *Hub) LookTasks(project string) { d.Looked = append(d.Looked, project+"/tasks") }

// Wake answers as a hub would, recording the ask where a test can read it.
func (d *Hub) Wake(project, agent string, topic machine.Topic) {
	d.Woke = append(d.Woke, string(topic))
}

// WakeProject answers as a hub would, recording the ask where a test can read it.
func (d *Hub) WakeProject(project string, topic machine.Topic) {
	d.Woke = append(d.Woke, string(topic))
}

// WakeAll answers as a hub would, recording the ask where a test can read it.
func (d *Hub) WakeAll(topic machine.Topic) { d.Woke = append(d.Woke, string(topic)) }

// WakeRuns answers as a hub would, recording the ask where a test can read it.
func (d *Hub) WakeRuns(topic machine.Topic) { d.Woke = append(d.Woke, string(topic)) }

// StillSince is a reading of an agent whose screen stopped at a known instant — the SPELL a nudge
// is deduped against, which Saying leaves unset because its callers measure the dwell themselves.
func StillSince(word string, at time.Time) observe.Observation {
	o := Saying(word)
	o.StillSince = at
	return o
}
