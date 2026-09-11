## ADDED Requirements

### Requirement: A submit's questions are a state the agent sits in

Where a submit is taken through questions, the agent SHALL stand in an interviewing
state for the whole exchange. The state's action SHALL conduct the interview under a
context the machine cancels on the way out, so the sequence of questions is one
process rather than a position reconstructed on each call.

An agent SHALL be given as long as it needs to answer: taking time to research is
the behaviour the questions exist to provoke. The hub SHALL re-pose a question when
the agent is observed at an empty prompt after that question was put, since an agent
at a prompt has ended its turn.

If the agent's working tree changes while the interview is open, the agent SHALL
return to working and the answers SHALL be discarded, because they describe a tree
that no longer exists. The tree SHALL be examined when an answer arrives and when a
question is re-posed, and never on the machine's beat.

#### Scenario: The agent researches between answers

- **WHEN** an agent spends several minutes reading code before answering a question
- **THEN** it stays in the interviewing state undisturbed, because its session is
  reporting a turn in progress rather than an empty prompt

#### Scenario: A dropped question is put again

- **WHEN** an agent is observed at an empty prompt after a question was put to it
- **THEN** the hub re-poses that question

#### Scenario: Editing code abandons the interview

- **WHEN** an agent changes its working tree while questions are outstanding
- **THEN** it returns to working and the submit is abandoned, so no submission
  describes a tree that has since moved

#### Scenario: An abandoned interview is visible

- **WHEN** an agent stops answering a half-finished interview
- **THEN** the board shows it interviewing, with the dwell that state has stood for,
  rather than showing it working

### Requirement: A merge is a state the hub waits in

A human's merge SHALL be recorded as an intent against the pull request and
announced. The machine SHALL move the pull request into its merging state and run
the merge there, and the result SHALL move it on — landed, conflicted, or back to
approved when the merge did not run.

The merging state SHALL also be left by observing the world, so a merge whose result
is lost still settles: a base already carrying the branch is merged, and a conflict
recorded against the pull request is a conflict, whatever the action returned.

#### Scenario: A merge asked for survives a restart

- **WHEN** a human asks for a merge and the hub restarts before it completes
- **THEN** the intent is still recorded, and the pull request is recovered through
  the merging state's own orphan exit rather than by a startup sweep

#### Scenario: A conflict reaches the author through the pull request

- **WHEN** a merge hits a conflict
- **THEN** the conflict is recorded against the pull request, and the author's own
  map observes it there

## MODIFIED Requirements

### Requirement: The orchestrator sequences a session reset itself

The orchestrator SHALL issue a harness command, wait for its answer, and then deliver the following
instruction itself through the ordinary delivery path, wherever it needs an agent's session reset —
cleared, compacted, or moved to another model — before that instruction lands. It SHALL NOT hand the
instruction to the harness command to be sent on its behalf.

A harness command that fails or times out SHALL leave the orchestrator holding the decision about
what happens next, and the agent SHALL NOT be left both un-reset and un-instructed.

A reset SHALL be a state the agent stands in for the duration, with exactly one
implementation. An agent that asks the hub for its next action while a reset is
running SHALL be told the state it is standing in, which is the honest answer: the
reset is in progress, and the state it leads to hands the work over with its own
brief.

#### Scenario: A cleared agent is instructed by the orchestrator

- **WHEN** the orchestrator clears an agent's session as part of handing it work
- **THEN** it waits for the clear to answer, and then delivers the agent's directive itself

#### Scenario: A failed reset is the orchestrator's to answer for

- **WHEN** the orchestrator clears an agent's session and the command answers with a failure
- **THEN** the orchestrator decides what follows — retrying, escalating, or leaving the agent as it
  was — rather than the failure being recorded where no caller reads it

#### Scenario: An agent asking during a reset is told where it stands

- **WHEN** an agent asks the hub for its next action while its session is being reset
- **THEN** it is told it stands in the resetting state, and the state it leads to
  delivers the work when the reset lands

#### Scenario: Nothing asks about a pending reset

- **WHEN** an agent asks the hub for its next action
- **THEN** it is never answered with a notice that a reset is about to land, because a reset in
  progress is inside a call that has not yet returned

#### Scenario: One implementation of a reset

- **WHEN** a session is cleared or moved to another model, whatever prompted it
- **THEN** it happens in that role's own clearing or retiering state, because no
  other path performs a reset
