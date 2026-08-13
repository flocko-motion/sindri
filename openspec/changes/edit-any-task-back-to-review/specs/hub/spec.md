# hub — delta

## ADDED Requirements

### Requirement: A planner may revise any task, and every revision returns it for a verdict

A planner SHALL be able to edit any task it can see, whether or not the user has approved it. The
task's existence SHALL be the only precondition.

Every edit that lands SHALL return that task to awaiting-review. Approval is the user's record that
they have read the task; an edit makes that record untrue, so it is cleared. It is not permission
the planner has to hold — neither approval nor priority is a mechanism guarding the hub against its
own agents, and reading them as one is what made this edit a refusal.

The rule SHALL be that one, applied to every edit. There SHALL be no split between fields whose
revision returns a task for a verdict and fields whose revision does not: which of them are
"substantive" is a classification nothing tests and every field added later drifts from.

Returning a task SHALL also hold it out of the claim pools, since each requires approval — a task
whose definition has just changed SHALL NOT be handed to a worker before the user has seen the
change. It SHALL NOT reach a worker already holding that task: the claim gate decides what is
handed OUT, and a holder finishes and submits exactly as it would have.

That holds inside a feature as well as outside one, and it SHALL be answered by asking whether
anything under the feature is still open rather than whether anything is claimable. The two
questions differ exactly where a gate is shut, so a query written for assignment reports gated work
as ABSENT, which cannot be told apart from finished: a feature was declared complete, its worker
told to put the branch up, and the feature closed over a subtask nobody had worked. A feature with
work under it awaiting the user, at any depth, SHALL NOT be reported as finished and its branch
SHALL NOT be accepted as a pull request. Having nothing to hand out and nothing finished, the hub
SHALL make the worker WAIT for the user's verdict, as it does for any other empty queue.

Every edit that lands SHALL be recorded on the task itself, carrying the value each changed field
held before it, so the user can see what changed rather than only that something did. Where the
verdict being cleared was a rejection, its reason SHALL be carried into that record: the reason is
held in the approval state and nowhere else, and this edit is the write that clears it.

Where an agent holds the edited task, the hub SHALL tell it directly: name what changed, point at
the task where the change is recorded in full, and say the work is still its own to finish.

What an edit changed SHALL be read from the task as stored either side of the write, never echoed
back from the request. Where nothing moved — the values given are already held, or the field
belongs to the task's own source — nothing SHALL be recorded, the user's verdict SHALL stand, and
the caller SHALL be told which of the two it was.

Editing SHALL NOT set a task's priority. An edit returns the task for a verdict and a re-ordering
leaves the verdict standing, so the verb SHALL name the ordering verb rather than carry a field
whose meaning would then depend on which verb reached it.

A user's own edit of a task SHALL NOT return it for a verdict: the user editing a task is the user
reading it.

#### Scenario: Editing a task the user has already approved

- **WHEN** a planner edits an approved task
- **THEN** the edit is applied and the task returns to awaiting the user's approval

#### Scenario: Every edit returns the task, whatever it touched

- **WHEN** a planner edits any field of a task — its title, body, type, labels or parent
- **THEN** the task returns to awaiting the user's approval, the same for every field

#### Scenario: An edited task is not handed out again

- **WHEN** a task that was approved and rated is edited
- **THEN** no worker is offered it until the user has ruled on the change

#### Scenario: A feature is not finished over work awaiting the user

- **WHEN** a subtask of a feature a worker holds is edited, and every other subtask is done
- **THEN** the worker is not told the feature is finished, its branch is refused as a pull request,
  and the refusal names the subtask that awaits the user

#### Scenario: The held feature waits for the verdict

- **WHEN** a held feature has nothing workable left and work awaiting the user under it
- **THEN** the worker waits, and the user's approval hands it that subtask

#### Scenario: The worker holding it keeps the work

- **WHEN** the task a worker is working on is edited
- **THEN** the worker is still directed to finish and submit it, and is told what changed, where
  the full record is, and that the work remains its own

#### Scenario: The change is recorded where the user reads it

- **WHEN** an edit lands
- **THEN** the task's own thread carries what each changed field held before the edit and what it
  holds now

#### Scenario: Revising a rejected task keeps the reason it was rejected for

- **WHEN** a planner edits a task the user rejected with a reason
- **THEN** the record on the task carries that reason, and the task awaits a fresh verdict

#### Scenario: An edit that writes nothing spends no verdict

- **WHEN** a planner edits a field of a task whose own source owns that field, or supplies values
  the task already holds
- **THEN** nothing is recorded, the user's approval stands, and the caller is told nothing changed

#### Scenario: Rating is not editing

- **WHEN** a planner passes a priority to the edit verb
- **THEN** it is refused, nothing is written, and the ordering verb is named

## MODIFIED Requirements

### Requirement: A planner may order work, never authorise it

A planner SHALL be able to express the sequence of the work it plans: to propose a priority on a
task it creates, and to change the priority of a task afterwards.

No planner action SHALL decide, on the user's behalf, that work is released or that released work
is withdrawn. A task is claimable when it carries a priority AND the approval gate is not holding
it, so the rule is one boundary rather than a list of cases: a planner MAY move a priority between
values, and MAY rate a task the gate still holds; it SHALL NOT cross between unrated and rated on a
task the gate would let out, because setting a first priority there releases work and clearing one
withdraws work already released.

A planner MAY return a task to the user for a fresh verdict by editing it, which suspends that
task's release until the user rules. That is the one planner action reaching the gate at all, and
it is permitted because it hands the decision back rather than taking it: the user is asked to look
again at a task that no longer says what they read.

A refusal SHALL give that reason, so a planner can tell a rule from a fault.

Where a proposal is created carrying a priority, it SHALL NOT be claimable at any point between its
creation and its gating — the ordering of those writes is part of the guarantee, not an artefact of
it.

A rating a planner makes SHALL NOT be announced to workers as available work, since by the rule
above it never makes any.

An interface offering the human the approval of a rated task SHALL show the priority it carries, so
the sequence being authorised is visible at the moment it takes effect. An interface offering an
action the caller may not perform SHALL NOT offer it: the flow that invites a human to approve a
task they have just rated is a human flow, and a planner SHALL NOT be led into it.

#### Scenario: Rating a proposal

- **WHEN** a planner sets a priority on a task still awaiting approval
- **THEN** the priority is recorded, the task remains unclaimable, and the planner is told so

#### Scenario: Re-ordering approved work

- **WHEN** a planner changes the priority of an approved task that already carries one
- **THEN** the new priority is recorded and the task remains claimable, as it was

#### Scenario: Rating an approved task that has none

- **WHEN** a planner sets a priority on an approved task carrying none
- **THEN** it is refused, nothing is written, and the reason given is that it would release the task

#### Scenario: Clearing the priority of approved work

- **WHEN** a planner would clear the priority of an approved, rated task
- **THEN** it is refused, and the reason given is that it would withdraw released work

#### Scenario: A proposal created with a priority

- **WHEN** a planner creates a task carrying a priority
- **THEN** the task awaits approval and no worker can claim it

#### Scenario: Approving a task a planner rated

- **WHEN** the human approves a task that carries a priority
- **THEN** that priority is named as part of the approval, and no further rating is asked for

#### Scenario: Re-ordering leaves the verdict standing

- **WHEN** a planner changes the priority of a task the user has approved
- **THEN** the approval is untouched and the task is not returned for a fresh verdict
