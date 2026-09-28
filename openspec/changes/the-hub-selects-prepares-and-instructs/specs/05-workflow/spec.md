# 05-workflow — delta

## ADDED Requirements

### Requirement: The hub selects, prepares, and instructs, in that order

Handing work to an agent SHALL happen in three steps, in one order, for every unit of work.

**Selecting** is the hub choosing which unit an agent will work and recording that it holds it. It is
the hub's own decision: it reaches nothing outside the hub and SHALL tell the agent nothing. Holding
the work from this moment is what protects it while the steps behind it run — an agent whose pod is
down or restarting still holds what it was given.

**Preparing** acts on the agent's session for the work already selected: its context is cleared, then
it is put on the model that work's tier dispatches to. Each step SHALL run for every selection rather
than being asked for, since each is already its own no-op when there is nothing to do — a clear of an
empty session, a switch to the model already running.

**Instructing** is the hub telling the agent what it holds. It SHALL be the last step, so nothing is
ever said into a session that is still being prepared, and it is the first the agent hears of any of
it. The words SHALL describe what the agent holds, so an agent hears the same brief however the work
came to be selected for it.

No step SHALL lead back to a step already passed. A state an agent RESTS in SHALL NOT lead into
preparation: preparation follows a selection, and reached from rest it is a way back into itself. A
worker circled `between-subtasks → retiering → assigning → between-subtasks` for four days at
twenty-six seconds a lap, because a preparation step that could not settle was watched from the state
the agent rested in.

#### Scenario: A selection tells the agent nothing

- **WHEN** the hub selects a task or a subtask for a worker
- **THEN** the work is recorded as held and the worker's session is told nothing, because what
  happens to that session next has not run yet

#### Scenario: The session is prepared before the agent hears

- **WHEN** a worker has had work selected for it
- **THEN** its session is cleared and put on the work's model, and only then is it told what it holds

#### Scenario: Preparation runs without being asked for

- **WHEN** work is selected for a worker whose session is empty and already on that work's model
- **THEN** both preparation steps run and each does nothing, and the worker is instructed as it would
  be after a clear and a switch that had work to do

#### Scenario: Rest cannot re-enter preparation

- **WHEN** a worker stands in a state it rests in — idle, or between the subtasks of a feature
- **THEN** the only work step it can be moved to is a selection, so no sequence of moves returns it to
  a preparation step it has already passed

#### Scenario: A hub restart mid-chain leaves the agent at its work

- **WHEN** the hub restarts, or an agent's pod goes away, while that agent is being prepared or
  instructed
- **THEN** the agent stands at the work it already holds and is told what that is on its next ask,
  rather than being returned to a state that holds nothing

### Requirement: A preparation step that does not land stops the agent for a human

Each preparation step SHALL either land or stop the agent. Where the harness reports that a clear or
a model switch did not land, the hub SHALL escalate to the user, naming what it was doing and what
the harness said, and SHALL NOT instruct the agent. The work SHALL stay selected and the session
SHALL be left with nothing said into it.

Carrying on is refused because each way of carrying on has a known outcome. Past a failed clear, the
model step types `/model` into a session that still holds its history, which opens a confirmation
dialog and swallows the brief meant to follow it. Past a failed switch, the work runs on whatever
model the session happened to hold, under a hub that believes it chose. Typing a line into a pane is
not a thing that fails, so one that did is a fault in the harness rather than a turn the flow takes.

#### Scenario: The clear does not land

- **WHEN** the harness reports that it could not empty a worker's session for its selected work
- **THEN** the worker is escalated with the harness's own reason, holds its work still, and the model
  step behind the clear does not run

#### Scenario: The switch does not land

- **WHEN** the harness reports that it could not put a worker's session on its work's model
- **THEN** the worker is escalated with the tier and the harness's own reason, holds its work still,
  and has been told nothing about that work

#### Scenario: A tier with no model

- **WHEN** work is selected whose tier the coding-agent backend does not recognise
- **THEN** the switch is refused rather than guessed at, and the worker stops the same way

### Requirement: A selection that cannot be made stops the agent for a human

A selection SHALL either land or stop the agent. Selecting reads the project's repository and writes
to the store, so it can fail for reasons outside the flow; where it does, the hub SHALL escalate to
the user, naming the unit and the reason it was given. The unit SHALL stay open for whoever is free
once the cause is fixed, and nothing SHALL be said into the agent's session.

Retrying is refused because a selection fails the same way every time: the reference branch is
missing, the main checkout has no branch at all, or the store would not take the write. A worker span
in `worker/assigning` once every two seconds for an hour over a checkout left on a tag, with its
state log the only thing that said so.

#### Scenario: The claim cannot be made

- **WHEN** the hub tries to claim a task or a subtask for a worker and the claim returns an error
- **THEN** the worker is escalated with the unit and that error, the unit stays open, and the worker
  is not returned to a state that would select the same unit again

#### Scenario: The project has no reference branch to work from

- **WHEN** a project's main checkout is on no branch at all, so the branch that would BE its
  reference does not exist
- **THEN** that is what the escalation says, rather than being retried on every beat

### Requirement: An action's outcome moves the subject, whatever else it reported

An action SHALL be able to report a failure AND say where the subject belongs, and the flow engine
SHALL honour both: the outcome moves the subject and the failure is the reason on the record. Only an
action that names no outcome at all SHALL leave the subject where it stands, to be reached by that
state's conditions.

A state the hub acts in is left by its action's outcome. Dropping the outcome because an error came
with it leaves the subject inside that state with no edge out of it, and the action is started again
on every poll — an agent stuck for as long as the cause lasts, saying nothing to anybody. The engine
SHALL NOT move a subject on an outcome from an action whose own run was cancelled, since the subject
has already left the state that answer is about.

#### Scenario: An action fails and names where that leads

- **WHEN** an action returns an outcome together with an error
- **THEN** the subject moves where its state says that outcome leads, and the error is recorded as
  the reason

#### Scenario: An action fails with nothing to say

- **WHEN** an action returns an error and no outcome
- **THEN** the subject stays where it is, and its state's conditions are what move it

#### Scenario: An action answers after the subject moved

- **WHEN** an action returns after the subject was moved out of the state that started it
- **THEN** its outcome moves nobody

### Requirement: The hub tells an agent what to do; it never sends it to ask

The hub SHALL NOT tell an agent to ask for its directive. Where there is something the agent should
be doing, the hub SHALL say it; where there is not, the hub SHALL say nothing about it. This holds
wherever the hub speaks — a reply to a verb, a refusal, a push — because the hub is holding the
answer at the moment it speaks.

The one exception is a message whose TIMING is the agent's to choose. Mail is read when the agent
decides to read it, so telling an agent that mail is waiting and leaving the reading to it states
something the hub cannot state for it. A directive carries no such choice: the agent has no say in
what it is working on.

Sending the agent to fetch costs a call and a turn for an answer already in hand, and the fetch can
be lost. A worker was told `pr-sd-b46516 merged. Run \`sindri\` for your next task`, its pane was
interrupted before it made that call, and it sat there while the hub counted the work dispatched.

#### Scenario: A refusal states what is refused

- **WHEN** an agent runs a verb that does not apply where it stands
- **THEN** the reply names what it refuses and what the agent should do instead if there is anything,
  and points at no bare `sindri` call

#### Scenario: An agent with nothing to do is told so and left alone

- **WHEN** an agent holds no work and reads the backlog or its mailbox
- **THEN** it is told that it holds nothing and that work reaches it when there is some, rather than
  being sent to ask for work

#### Scenario: Mail keeps its pointer

- **WHEN** mail arrives for an agent that is busy or away
- **THEN** it is told that mail is waiting and left to read it when it chooses

### Requirement: No state holds an agent that has nothing to be there for

A state that only a condition can move an agent out of SHALL declare where an agent goes that does
not hold what the state exists to hold. A state whose action always lands it somewhere is exempt by
its shape: its outcome moves the agent whatever the world says.

Every exit of a work state reads the work it assumes is in hand, so an agent that arrives with none
matches nothing and stays for as long as the hub runs — shown as busy on the board the whole time.
This is reachable: an escalation raised BEFORE anything was claimed resolves to the state a working
agent resumes into, and a worker landed in `worker/working` holding neither a task nor a feature.

An agent may legitimately stand in such a state holding nothing where a pull request is what anchors
it — an author whose state row was cleared still owes its reviewer an answer. Each exemption SHALL
name why holding nothing is legitimate there.

#### Scenario: A work state is entered with nothing held

- **WHEN** an agent reaches a state that means it is at work, holding neither a task nor a feature
- **THEN** it is moved to the state that means it holds nothing, rather than being kept there

#### Scenario: An escalation raised before a claim

- **WHEN** a worker escalates from the selection step, where nothing has been claimed, and the user
  then clears that escalation
- **THEN** the worker ends at the state for holding nothing, whatever intermediate state the
  escalation returns it to

#### Scenario: A pull request is what it holds

- **WHEN** an agent holds no task but is answerable for a pull request it filed
- **THEN** the states that exist for that pull request keep it, because the pull request is what they
  are anchored on

### Requirement: The model step types the switch alone

The step that puts a session on its work's model SHALL type only that switch. It SHALL NOT clear the
session first, because the step before it has already done so and a clear that did not land stops the
agent there.

A second clear is refused on evidence: a freshly cleared session reports a few tokens rather than
none, the wait that follows a clear ends when the reading FALLS, and a reading that is already at the
floor never falls again. That wait runs to its cap and reports a failure, which stops an agent whose
session was ready all along.

The standalone model change a human asks for SHALL still clear first, since nothing has prepared that
session and `/model` on cached history opens a dialog.

#### Scenario: A switch onto a session just cleared

- **WHEN** a worker's session has been emptied by the preparation step and is then put on its work's
  model
- **THEN** exactly one clear has been typed into that session, by the step whose job it was

#### Scenario: A human changes a working agent's model

- **WHEN** a user changes the model of an agent whose session holds its history
- **THEN** that session is cleared and then switched, because nothing else has prepared it

## MODIFIED Requirements

### Requirement: A worker past its context threshold is cleared and assigned, not refused

The hub SHALL select a leaf task or a container for a worker whose current context size is at or past
a fixed threshold exactly as it would for any other worker. Once selected, the worker's session SHALL
be cleared — its live context discarded, Claude Code's own `/clear` — rather than compacted, before
the worker is instructed: a session that far past the threshold is not worth summarizing. A worker
with no context measurement yet SHALL NOT be treated as past the threshold.

Selecting here means the hub deciding which unit the worker will hold and recording it, which tells
the worker nothing (-> "The hub selects, prepares, and instructs, in that order"). The clear runs
between that decision and the instruction, which is what makes it possible at all.

The order is fixed: the session is cleared, then put on its work's model, then the worker is
instructed. Both preparation steps run every time, so a worker that is past the threshold AND due a
model switch gets each of them once, in that order.

#### Scenario: A full worker is selected for and then cleared

- **WHEN** an idle worker whose context is past the threshold asks for its next task, and an open
  leaf or an unheld container exists
- **THEN** the hub selects it for the worker, clears the worker's session, and then instructs it,
  rather than refusing the selection

#### Scenario: A full worker is not left waiting on a human

- **WHEN** an idle worker whose context is past the threshold asks for its next task
- **THEN** it is not told to wait for a human to clear it; the clear fires automatically as part of
  being handed the task

#### Scenario: An unmeasured worker is selected for normally

- **WHEN** an idle worker with no context measurement yet asks for its next task
- **THEN** the hub selects for it exactly as it would for any worker under the threshold

#### Scenario: The clear and the model switch both run

- **WHEN** a worker is both past the context threshold and due a model switch for its tier
- **THEN** its session is cleared and then put on the tier's model, and it is instructed after both
