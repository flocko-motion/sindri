# The hub selects, prepares, then instructs

## Why

A worker on `ranke-ts` spent four days going round in a circle. Its state log holds the same three
moves, five hundred rows of them, one lap every twenty-six seconds:

    between-subtasks --tier-mismatch--> retiering --done--> assigning --holds-feature--> between-subtasks

It held a feature, its next child was rated `mid`, its session ran `claude-opus-5`, and it was never
handed the child. Two faults met there, and each is enough on its own.

**The record was read as proof of the session.** `SetModel` decided "already there" from the roster
row. A switch writes that row and then injects `/clear` and `/model`; when the injection is refused —
this one was, because the pane held an unsent line — the row keeps the model nobody set. Every later
switch then found the row agreeing and returned success without sending anything, while the tier
check judged the model detected off the live transcript. Two readings of "what is this agent running"
that could disagree, and nothing to notice they had.

**Preparation ran ahead of the selection, and led back into it.** Clearing the session and putting it
on the work's model were CONDITIONS, watched from the states a worker rests in. Rest could therefore
send an agent into preparation; preparation led to the claim; and the claim, for a worker that
already held a feature, bounced back to rest. Any preparation step that fails to settle closes that
loop. `machine.Look` bounds one pass at twelve, so an ask answered — it simply answered from a world
that never moved, and the fleet's beat re-entered the circle a few seconds later.

Underneath both is one unstated question: **when does an agent learn what it has been given?** The
claim actions answered it by accident. They claimed the work AND spoke its brief in a single step, so
preparation had nowhere to sit except in front of the claim, where it could only be a condition. The
order this proposal states was already the intended one — `preparation-clears-a-full-worker` says the
assigner "claims it for the worker, then clears the worker's session before its directive is
delivered" — but the word *claims* carries two readings. One is the hub deciding, privately, which
task an agent will get. The other is the agent being told. The code took the second, and the spec
cannot distinguish them.

## What Changes

The vocabulary comes first, because the rest follows from it. **Selecting** a task for an agent is the
hub's own decision and says nothing to anyone. **Preparing** acts on the agent's session. **Instructing**
is the single moment the agent learns any of it. The hub does all three, in that order, for every unit
of work.

- The claim actions stop speaking. `PickWork` and `PickSubtask` select and nothing more; delivery
  becomes its own step, `HandOver`, at the end of the chain. The brief is rendered there from the
  agent's own row rather than carried down from the claim.
- The worker's map becomes the linear chain `assigning | picking → preparing → retiering →
  handing-over → working`. Every edge inside it leads to a later step, so no step can return the
  agent to one it has passed.
- Clearing the session and setting its model stop being conditions and become steps, run for every
  selection. Each is already its own no-op when there is nothing to do, so nothing needs to ask
  first. `cond.SessionInTheWay` and `cond.TierMismatch` are deleted.
- A preparation step that does not land stops the agent and asks the user. Typing a line into a pane
  is not a thing that fails, so one that did is a fault in the harness. The work stays selected and
  nothing is said into the session.
- The model step types the switch alone, since the step before it emptied the session. `SetModel`
  keeps its own clear for the standalone change a human asks for. Leaving both to clear is what the
  sequential order exposed: a freshly cleared session reports a few tokens rather than none, the wait
  after a clear ends when the reading FALLS, and a reading already at the floor never falls — so the
  second clear ran to its five-minute cap and reported a failure on a session that was ready.
- `worker/clearing` becomes the human's armed clear alone, resting at idle both ways.
- `SetModel` judges "already there" by what the session runs. The roster row is the CHOICE, written
  whenever it changes because the next launch carries it; the transcript is the FACT, and only that
  decides whether anything needs typing.
- The whole tier-and-model question moves behind the harness as `TierIs` and `SetTier`. Which model a
  tier deserves and whether two ids name one model sat on opposite sides of a seam, so three callers
  joined them themselves, against three different readings. `Deps.ModelForTier` and
  `Harness.ModelMatches` come off the port.
- `situation.Situation.Model` is the one reading anything decides a model on, joined once by the
  gatherer from the roster row and the observation.

## Impact

- Supersedes the wording of the assignment requirement added by `preparation-clears-a-full-worker`
  (unarchived). That requirement is right about the order and ambiguous about the word; this delta
  restates it in the three-verb vocabulary. The two should be archived together.
- Source: `internal/hub/flow/agent/{act,cond}`, `internal/hub/flow/agent/roles/worker/*`,
  `internal/hub/flow/{fleet,task}`, `internal/hub/harness/{model,runtime}.go`,
  `internal/hub/world/{observe,situation}`, `internal/hub/core/core.go`, `internal/hub/wiring.go`,
  `internal/hub/prompts/*`.
- `DirContainerClaimed` is deleted. A feature's first subtask and every later one are handed over by
  the same step now, and `DirContainerWorking` already carried strictly more — the checkpoint loop,
  the comment budget, the tooling block.
- The test doubles learn the three tiers. `flowtest.Hub` and the hub package's `fakeAgent` both
  answered that no tier was recognised, which was harmless while the model step was a branch and
  fails every selection now that it is a step.
- Nothing changes for an agent that is working. The chain is entered by a selection and left at the
  work; an agent already holding a task is untouched by any of it.
