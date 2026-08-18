# Workflow — delta

## ADDED Requirements

### Requirement: An agent can escalate a decision that is not its to make

An agent that cannot go on without a decision only the user can make SHALL be able
to say so, stop, and be visible in having stopped. The capability SHALL be a pair of
verbs, available to every role, since any agent can meet such a decision: `escalate
<what needs deciding>` raises it and `resume` clears it.

The question SHALL be REQUIRED. An escalation that states no question tells the user
only that something is wrong, which is the part the board already shows them, so one
raised without a question SHALL be refused and SHALL record nothing.

The escalated state SHALL be durable, surviving a hub restart. An escalation that
evaporated would leave an agent refused by every verb that lands work with nothing
to explain why, which is the silent stuck agent this capability exists to prevent. No
unrelated write to the agent's workflow state SHALL clear it: only raising and
clearing an escalation SHALL change it.

The question SHALL be recorded where it can be read after the session that raised it
is gone: in the agent's activity log, and — where the agent holds work — as a comment
on that task, attributed to the agent, which is what a human or the next agent reads
when they open the work. A failure to record the comment SHALL NOT cost the agent the
escalation itself, since the state is what stops it.

Re-escalating while escalated SHALL replace the question. A question the agent cannot
re-put would leave it waiting on an answer to the wrong thing.

#### Scenario: An agent stops on a decision it cannot make

- **WHEN** an agent escalates, stating what needs deciding
- **THEN** it is marked as waiting on the user, the question is recorded in its
  activity log and on the task it holds, and it is told to wait rather than work
  around the question

#### Scenario: An escalation with no question

- **WHEN** an agent escalates without stating a question
- **THEN** the attempt is refused, nothing is recorded, and the reply says the
  question is what the user needs

#### Scenario: The escalation outlives the hub

- **WHEN** the hub restarts while an agent is escalated
- **THEN** the agent is still escalated and the question is still readable

#### Scenario: An unrelated state change does not release it

- **WHEN** an escalated agent's phase, task or branch is written for any other reason
- **THEN** its escalation stands untouched

### Requirement: An escalation holds every verb that lands work, in every role

While an agent is escalated, every verb that LANDS work SHALL be refused, whatever the
agent's role. Landing work is: taking work on, putting a branch or an openspec change
up, checkpointing, contributing, and ruling on a pull request. A planner's `openspec
submit` is a worker's submit in different dress, so one being refused SHALL mean both
are — a rule stated for one role and implemented for another leaves the same act open
under a second name, which is the failure this capability exists to prevent.

Each refusal SHALL quote the agent's own question back and SHALL name the verb that
clears the escalation, so the hold reads as the agent's own doing and points at the way
out rather than reading as a hub that has gone silent.

Reads SHALL NOT be refused. An agent handed a decision has to re-read the task, the
comments on it, and its own change to act on the answer — so its status, its task, a
pull request's diff, the pull request list, its log, and the read subset of git SHALL
stay available. Denying those makes the answer unusable.

Proposing SHALL NOT be refused either. Creating, editing, prioritising and reopening a
task each produce something the user rules on before it becomes work, so an escalated
planner tidying the backlog while it waits changes nothing anyone acts on. Nor SHALL
WITHDRAWING be refused: an agent that escalated on realising its submitted branch rests
on a guess needs the verb that takes that branch back, and refusing it would trap the
agent in the state its escalation was raised about.

Every message the hub sends about the hold SHALL describe it exactly, in one wording.
An agent acts on what the hub tells it, so "everything is refused" said where something
is not is worse than saying nothing: it spends the agent's trust in every other message.

The verb that clears the escalation SHALL never be refused while one is held.

The AGENT SHALL clear its own escalation, because only it knows whether it has
understood the answer. The USER SHALL be able to clear one as well, from the host: an
agent may be deleted, restarted, or simply wrong that it was blocked, and an
escalation nobody can clear is a stuck agent by another name. Clearing an escalation
that was never raised SHALL be a no-op. Clearing one SHALL be recorded, and SHALL
return the agent to the work it still holds rather than to a state of its own.

#### Scenario: A worker's landing verbs

- **WHEN** an escalated worker tries to submit, checkpoint, contribute, or claim work
- **THEN** the verb is refused with the agent's own question quoted and the clearing
  verb named, and the work is unchanged

#### Scenario: A reviewer's verdict is a landing too

- **WHEN** an escalated reviewer tries to approve or reject a pull request
- **THEN** the verdict is refused, since it lands work on the author's behalf

#### Scenario: A planner ships nothing while escalated

- **WHEN** an escalated planner tries to ship its openspec changes as a pull request
- **THEN** it is refused, in the same terms a worker's submit is

#### Scenario: A held verb refuses to run, not merely to be listed

- **WHEN** an escalated agent runs a held verb anyway
- **THEN** it is refused and nothing it would have changed has changed

#### Scenario: Proposing is not landing

- **WHEN** an escalated planner creates, edits, prioritises or reopens a task, or an
  agent withdraws a pull request it had submitted
- **THEN** each is served normally, since none of them lands work

#### Scenario: Reading the answer's context

- **WHEN** an escalated agent reads its task, a diff, the pull request list, or its log
- **THEN** each read is served normally

#### Scenario: The agent resumes itself

- **WHEN** an escalated agent has the user's answer and resumes
- **THEN** its escalation is cleared, the release is recorded, its work verbs are
  available again, and its directive is once more the work it holds

#### Scenario: The user releases an agent that cannot

- **WHEN** the user clears an agent's escalation from the host
- **THEN** the escalation is cleared and recorded as the user's doing

### Requirement: An escalated agent is told what it asked, and is not nudged

The hub's answer to an escalated agent asking for its next action SHALL state that it
is escalated and SHALL repeat the question it asked, ahead of whatever its role and
phase would otherwise be told. An agent relaunched while escalated remembers nothing
of asking: without being told, it tries to carry on and is refused by every verb that
lands work without knowing why.

An escalated agent SHALL NOT be nudged for going quiet. It is idle by instruction —
the hub told it to wait for an answer only the user can give — so a nudge complains
about the state the hub is holding it in, exactly as it did to a retired agent before
that state was exempted. This exemption SHALL cover a turn cut off mid-sentence as
well, unlike the other parked states, since a resumed turn has no verb left that
lands work.

#### Scenario: A relaunched agent knows where it stands

- **WHEN** an escalated agent is restarted and asks the hub what to do
- **THEN** it is told it is escalated, what it asked, and to wait for the answer

#### Scenario: Waiting quietly is not a stall

- **WHEN** an escalated agent's screen has stood still past the stall dwell
- **THEN** it is not nudged
