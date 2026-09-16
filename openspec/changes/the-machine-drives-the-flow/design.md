## Context

The hub runs four declared flow machines over one engine (`internal/hub/flow/machine`):
the agent flow, and one each for tasks, pull requests and queued runs. The engine is
complete. `State.Action` is "what the hub does while the subject is here", a
`Transition` carries only `On`/`To`/`Why`, actions run under a cancellable context
with no timeout, and `move` cancels a running action before leaving. That is a Moore
machine, and it is the shape this change enforces everywhere.

Three of the four machines are already close to it. The task flow has no state column
at all — a task's `status` *is* its state, so its many writers are inputs and there is
nothing to drift from. The run flow has one writer of `run_state`. The PR flow has one
writer of `pr_state`, with a single exception at the merge.

The agent flow is the outlier, and its defect has a specific origin: `machine.Run`,
the router that applies a verb's declared `Offer.To`, has no caller in the module.
Boot validates that every declared target names a real state (`machine.go:146`) and
nothing ever executes one. With the router unwired, each verb has to move the agent
itself, which is where roughly twenty-eight `SetState` calls come from, alongside a
second claimer per role (`ClaimNext`, `ReviewDirective`), a second session preparer
(`PrepareAssignment`, `FireClearIfArmed`), and a pod lifecycle that lives entirely in
sweeps. `tick_reference.go` labels three of those sweeps "A BACKSTOP".

The failure this was found through: a reviewer with a stopped pod was walked into
`reviewer/clearing` by the machine's own beat on `cond.ReviewWaiting`, which never
asks about liveness; the clear failed against a dead pod; `act.Failed` led to `Taking`
regardless; the claim erased the unclaimed-review row that both wake paths key on, so
nothing ever started the reviewer. The guarded path — `RequestReview` via
`freeReviewer`, which does check `s.Up` — had already declined to assign it.

## Goals / Non-Goals

**Goals:**

- One writer for every act performed upon an agent, enforced by an architecture test.
- Every wait the hub performs expressed as a state, so a hub that dies mid-wait
  recovers through the machine's own orphan handling.
- Actions outside the machine reduced to: write a fact, publish a topic.
- The remaining sweeps limited to polling task sources, which announce nothing.

**Non-Goals:**

- Changing what an agent is told, or what a human sees on the board. The same column
  is rendered; the change is who may write it.
- Converting the task or run flows. They already satisfy the invariant.
- Any staged or reversible migration. See Decisions.

## Decisions

### The machine is the only actor upon an agent; everyone else writes facts

A verb an agent types, an action a human takes, and an event arriving from a task
source all do the same two things: record their domain fact durably, then publish the
topic naming it. The machine's conditions read the world that fact changed and decide
what moves.

The guard is a call-site test over `SetState`, `AssignReview`, `CloseReviews`,
`ClaimLeaf` and `ClaimContainer`. *Alternative considered:* guard `SetState` alone,
which is narrower and unambiguous. Rejected because the reviewer was stranded by a
write to a review row with no state write at all — the narrow rule would have passed
the exact bug that prompted this. The line is not "state column versus domain row";
it is whether the write names the agent who will do a thing, which is the machine's
decision to make.

### Announcements are hints; requests are records

A topic carries no payload and no authority, per the engine's existing law: "A topic
may only SHORTEN latency. Every transition it accelerates must also be reachable by
its state's own poll." The acceptance test is direct — delete every `Wake` call and
the system must still reach the same states, only slower.

That leaves imperative commands needing a home, because a flag cannot express
"restart this agent" when the pod is already up. Those are recorded as requests
carrying the time they were asked for, in the shape `Observation.Launching` and
`Stopping` already have. State-shaped commands stay flags on the roster row, as
`Retired`, `Stopped` and `ClearArmed` already are.

`workflowDeps.Notify()` currently publishes `topic.SessionRead` to every subject for
every event — "the notify carries no meaning, so it reaches all of them and the
machine drops what it cannot use". Each action gains the topic that names what it
did. That is most of the wiring in this change, and by the criterion above none of it
is load-bearing.

### What an agent holds and where it stands are two facts

`SetState` wrote the whole row — task, branch, container and phase — which is why every verb that
recorded a fact also moved the agent: it had no way to write one without the other. Splitting it is
what makes "one writer" expressible at all. `SetHolding` records what an agent holds and leaves the
phase alone; `SetPhase` writes the phase and leaves the holding alone, and the machine is its only
caller. Both are enforced by `internal/arch/onewriter_test.go`.

The read shape stays whole — `AgentState` is what the board and the directive ask for — so only the
WRITE side is divided. That is the line the guard is drawn on: a caller that records a claim is
recording a fact, and a caller that names where the agent now stands is deciding.

### A pass is atomic per subject

The engine's comment said "every look happens on this goroutine, so a subject's state and the action
running for it are read in one well-defined turn". That stopped being true when `Look` started
running passes on the caller's goroutine. Two passes over one agent then each decided from the state
before the other moved it, and the observable result was an agent handed work by one pass and freed
by the next for holding none — the very defect this change exists to remove, in the engine rather
than in a verb.

Each subject now has a pass lock. A caller holding the line WAITS for its turn; the loop SKIPS a
subject somebody is mid-pass on and comes round again, which is the same bargain every topic makes
and keeps one slow action off the fleet's beat.

### The agent's flow state stays stored

*Alternatives considered:* derive it from what the agent holds, as a task's state is
derived; or a hybrid, deriving resting states and storing the ones with an action.
Both rejected. Derivation works for a task because a task is passive — it has no
actions and is never in the middle of anything. An agent is an actor, so it has to be
able to stand in `taking` or `clearing` and to be recovered there by `Orphaned`.
Deriving would erase the transition rather than make it, and with it the pass record
saying why the agent moved. The hybrid split on "has an `Action`", which is a category
the engine does not have: `working` is being in the middle of a task exactly as
`taking` is being in the middle of a hand-over.

### A verb declares no destination

`Offer.To` and `machine.Run` are deleted; `Offer` keeps the verb and its reason, used
for gating and for rendering the map. *Alternative considered:* keep `To` as a
declared expectation with a test asserting it agrees with the conditions. Rejected
because `machine.Would` already answers "where will this land" by following the
conditions against a world gathered once, so `To` is a second account of one truth —
and it has been wrong for its entire life without anyone noticing.

All 38 `Verbs:` declarations are on agent role states, so this touches nothing else.

### Every wait becomes a state

The search this implies found three gaps, arrived at independently of the reviewer
bug: launching a pod, stopping one, and merging. The first two live in
`Observation.Launching`/`Stopping` plus `checkStuckLaunch` in the watchdog sweep. The
third is stranger — `pr/merging` is declared, and its doc says "this state exists
rather than the merge being a moment inside a verb", while `Merge` writes `"merging"`
then `"merged"` straight through and nothing in the map ever targets it. `pr/stuck`,
reachable only from `Merging` via `Orphaned`, is therefore dead code, and
`ReconcileMergingPRs` is its stand-in.

Entering these states properly deletes the sweeps and makes the declared recovery
real. It also gains something the current shape cannot express: a merge asked for is
a recorded intent that survives a restart, where today it is a call that either
happens or is lost.

### A submit is a request, and the tree is named rather than committed

The questions now come before the commit, where they used to come after it. The answers describe the
tree the author is looking at, so the tree has to still be there when the last one lands — and a
commit taken up front made the questions a formality about work already written down. What holds the
attempt together is a request record (`submit_intents`) naming the tree by fingerprint
(`git.TreeFingerprint`, a name for a working tree the way a sha is a name for a commit).

The request is the fact `cond.SubmitAsked` reads and the interview's rows are the record of what was
asked: two things, because they have different lives. The request is taken back the moment the
submit is taken; the rows outlive it, since they are what the reviewer reads on the pull request.

### The submit questionnaire is a state running a process

The agent stands in `worker/interviewing`, whose action conducts the whole exchange
under the machine's cancellable context. *Alternative considered:* a counter inside
`working`, which hides a distinct situation and makes the board show an agent working
while it waits on questions. The process form gives the question sequence as one
function, and cancellation as the cleanup — code moved, escalation, retirement, the PR
scrapped underneath all discard a half-finished interview with nothing to reset, and
the cancel is recorded.

What the process owns is the SEQUENCE, not the delivery. Each question is pushed as
its own message and the verb that takes an answer puts none of its own, so there is
exactly one asker: a verb that also asked would be a second one racing the process for
which question is outstanding. This was first written the other way round — pushing was
listed as a rejected alternative, for splitting every exchange into two messages and
making the interview depend on delivery landing — and both costs are real and were
accepted. A re-posed question has no command to ride back on, so push is the only path
that serves every question rather than all but the first, and one path is worth the
second message. The delivery risk is what the re-posing below answers: a question that
never landed leaves the author at an empty prompt, which is precisely the reading that
puts it again.

Two details decided here. The reminder fires when the agent is **observed at an empty
prompt** after a question was put, rather than on a dwell: an agent researching has a
changing pane, and keying on time in state would nag exactly the agent behaving well.
The observation must be newer than the question, or a stale reading re-poses
instantly and forever. And the working-tree check runs when an answer arrives and
when a question is re-posed — never as a polled condition, because a `git` exec per
agent per beat is what the standing observer exists to prevent.

### An action that waits on its subject declares it

The interview is the first action whose answer can only come from the agent it is about, and that
inverts the engine's one assumption about a caller. `Look` runs a state's action on the caller's
goroutine and waits behind one already in flight — deliberately, so "an asker gets the answer AFTER
the world moved". An agent typing bare `sindri` goes through `Look`, so an interview started there
would block on an answer that can only arrive from the call now blocked. A deadlock, not a delay.

`machine.Action.Awaits` declares it: such an action always runs on its own goroutine, and nobody
holding the line waits behind one. *Alternative considered:* bound the wait with a timeout, which
buys the same liveness. Rejected because the bound would be a number chosen by feel standing in for
a fact the action already knows about itself — and the two cases differ in kind, not in duration.

### Repairs are conditions, and the category goes

A condition is "you hold this, the world says otherwise, move" — which is what a
repair is. `cond.TaskGone`, `cond.FeatureGone` and `cond.ReviewOvertaken` are already
repairs; nobody calls them that. `HealSplitHierarchies` turns out to duplicate
`cond.TreeSplit` and `act.Yield` almost word for word, differing only in running at
boot rather than whenever the split is true.

*Alternative considered:* keep sweeps for repairs spanning several subjects.
Rejected: a cross-subject repair is several subjects each observing one fact, and
writing it as a sweep is how it becomes the only thing that notices.

### Lifecycle states are built by factories

Every role gains the full lifecycle, so the machine reaches every role for a clear
and a launch. Where a lifecycle state leads differs by role; what can move a subject
out of it does not — and the reviewer's `clearing` lacking `cond.SessionGone` while
the worker's has it is precisely how this bug got through.

A factory takes a struct of destinations with required fields, so an omitted edge
fails to compile. *Alternative considered:* hand-written states plus a declared
trigger set and a test over it, in the idiom of `act.All` and `cond.All`. Rejected in
favour of the compiler, on the same reasoning that makes `observe.State` an integer:
"the words exist in exactly one place". This also puts the shared prose in one place,
where today the same mechanism is described twice in two roles' words.

### One change, no staged transition

*Alternative considered:* land the architecture test first with an allowlist naming
every current writer, and empty it over time — the device `arch/situation_test.go`
already uses. Rejected: a ratchet that can sit half-pulled is exactly what produced
this, and an unfinished migration with nothing tracking the remainder is the failure
being fixed. The test lands in the same change as a gate, and the change is done when
it is green with no exemptions.

For the same reason there is no interim fix for the reviewer hole. The `Alive: false`
test that pins it is written inside this change rather than ahead of it.

### Tests stay synchronous, with one exception

Nothing today calls `.Reconciling()` except `hub.go:140`, so the beat exists only in
production and every test is its own scheduler. With one writer and fixtures that can
only build reachable states, there is nothing left to race. The exception is a single
test that runs the real loop and asserts a stopped agent with work waiting ends up
started, because the specific thing that failed here was the loop reaching a subject
nobody expected it to reach.

Fixtures lose their exemption entirely. `flowtest.AssignReviewer` builds a reviewer
holding an assigned review with its pod down — a world the machine would never
produce — and the tests standing on it pass. Fixtures now write the domain fact and
call `Look`, which stays deterministic because `Look` runs on the caller's goroutine
until the subject settles.

## Risks / Trade-offs

- **The diff is large and lands at once.** → The architecture test is the gate: the
  change is unfinished until it passes with no exemptions, so a partial conversion
  cannot be merged and forgotten, which is the failure mode being fixed.

- **Every fixture in the fleet and workflow packages is rewritten.** → Unavoidable
  given the fixtures are the bypass. It is also the point: a rewritten fixture that
  cannot reach a state proves the machine would not have produced it.

- **`machine.Would` becomes the only answer to "where will this land".** → It already
  gathers the world once and follows the conditions as a real pass does, and it is
  exercised by `Standing` on every command surface render.

- **Deleting `machine.Run` removes any route for forcing a subject into a state.** →
  Intended. A caller that wants a subject somewhere records the fact that puts it
  there.

- **The interview's working-tree check costs a `git` call.** → Bounded to answer
  arrival and re-posing, never the beat.

- **A stale observation could make the interview re-pose forever.** → The condition
  requires the agent seen at a prompt *since* the question went out, using the
  `TakenAt`/`StateSince` stamps the observation already carries.

- **`pr/merging` gaining a real action changes merge timing.** → The state is left by
  observation as well as by outcome, so a lost result still settles: a base already
  carrying the branch reads as merged, a recorded conflict reads as conflicted.

## Migration Plan

The hub has no users and no back-compatibility obligation, so this is a rebuild
rather than a migration. Order of work, each step leaving the tree building:

1. Lifecycle factories and the states they build, added to all four role maps.
2. `act.Launch`/`act.Stop` and their states; delete `FireIdleStops`,
   `FireIdleStarts`, `wakeAReviewer`, `FireArmedClears`, `checkStuckLaunch`'s bound.
3. Liveness folded into the claim conditions; the `Alive: false` regression test.
4. `pr/merging` entered; `ReconcileMergingPRs` deleted.
5. `worker/interviewing` and `act.Interview`.
6. Verbs reduced to fact-and-publish; `Offer.To` and `machine.Run` deleted; each
   action given the topic that names it; `CmdState` and `legacy()` deleted.
7. Parallel claimers and preparers deleted.
8. Repairs expressed as conditions; the remaining writing sweeps deleted.
9. Fixtures converted; the architecture test added and made green.

### An action restarts freely after a Stay; `ran` only answers "was this mine"

Confirmed against the engine. `m.ran[subject+"\x00"+action]` is written once in `begin`
and never cleared, and the ONLY reader is `orphaned`: an acting state entered before this
process started, with nothing running and no entry here, was left behind by a hub that
died. Whether an action may START is a different question, guarded on `m.act[subject]`
alone — the in-flight record, deleted the moment the action returns. So a `Stay` outcome
leaves the subject where it is with nothing running, and the state's next pass starts the
action again. `worker/stalled`'s prod relies on exactly that, and the interview may.

The cost of `ran` never resetting is bounded and correct: once this process has run an
action for a subject, that subject can never be reported orphaned in it again, because
this process is the hub that ran it.

## Open Questions

None left open. All three were settled while building, and are recorded here rather
than deleted, because what a question was and how it went is worth more to a later
reader than a heading that was quietly emptied.

- *Whether `escalated` takes its verb list as a factory parameter or stays
  hand-written.* **A parameter.** `lifecycle.Escalated(name, back, alsoAllowed)` takes
  the role's own additions positionally, beside the four every role shares, so a role
  that forgets one does not compile.
- *Where the merge intent is recorded.* **Its own column on the pull request row**
  (`prs.merge_asked`, via `SetMergeAsked`), kept off the shape the board renders: it
  is an instruction to the hub, and what a reader looks at is the state it leads to.
- *The dwell `act.Prod` dedups on.* **No dwell.** It keys on the idle SPELL the
  observation carries (`Spells.First` against `obs.StillSince`), so a stall is named
  once however many things notice it, and a pane that moves and stops again is a new
  spell rather than one already prodded for. The number chosen by feel is gone rather
  than argued over.
