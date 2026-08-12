# hub — delta

## MODIFIED Requirements

### Requirement: Planner task proposals are gated on user approval

A planner SHALL propose backlog tasks with `create-task`, but a proposed task
SHALL NOT be claimable by any worker until the user approves it. The hub SHALL
record a per-task approval state — pending, approved, or rejected — held in
`hub.db` separate from the task's own status. A task with no approval row is a
normal, claimable task; a task flagged pending or rejected SHALL be hidden from
the work an agent can claim. Approval and rejection SHALL be user-only actions
(`sindri task approve`/`reject`), and the hub SHALL inject the verdict into every
running planner's session.

Approval is not the only gate: an approved task with no priority is claimable by
nobody. So every front-end SHALL offer the priority on the task just approved,
in the same flow, whenever no rating already releases that task — its own or an
ancestor's. The two SHALL remain separate operations, chained rather than merged,
so there is one approve path and one priority path.

#### Scenario: Proposed task is withheld until approved

- **WHEN** a planner runs `create-task`
- **THEN** the task is created in the backend flagged pending the user's approval,
  and no worker can claim it while it is pending

#### Scenario: User approves a proposal

- **WHEN** the user approves a planner-proposed task
- **THEN** the approval gate clears, the task becomes claimable by a worker, and
  any running planner is told it was approved

#### Scenario: User rejects a proposal

- **WHEN** the user rejects a planner-proposed task with a comment
- **THEN** the task stays hidden from workers and the comment is injected into any
  running planner's session

#### Scenario: Approving an unrated task offers the rating

- **WHEN** the user approves a task that no priority releases
- **THEN** the priority for that task is offered in the same flow, and the user is
  told that until it is set no worker can claim the task

#### Scenario: Approving an already-rated task asks nothing further

- **WHEN** the user approves a task carrying a priority, or one below a task that
  carries one
- **THEN** the approve completes without asking for a rating it does not need

## ADDED Requirements

### Requirement: A priority reaches as far as the user says, and says what that does

Setting a priority SHALL take a scope: the named task alone, every open task below
it carrying no priority, or every open task below it. The narrowest SHALL be the
default, and SHALL be what a caller that names no scope gets. An unrecognised scope
SHALL be refused rather than narrowed, so no caller is told a cascade happened when
none did. A cascade SHALL NOT touch a task that has ended — its rating decides
nothing, and rewriting it would alter the record of work already done. Within one
cascade the tasks below SHALL be written before the task itself, since it is the
parent's rating that releases a package.

Every front-end offering the wider scopes SHALL first state what carrying the rating
down would do to the tasks below THIS task, and the two cases SHALL NOT be described
alike: while the parent is open its children are claimed as one package and their
ratings only order the subtasks handed out; once the parent has ended each open child
below it is claimed on its own, and its rating is what makes it claimable at all. No
front-end SHALL present a scope as releasing work that is not in fact claimable.

#### Scenario: The default scope leaves the tree alone

- **WHEN** the user rates a task without naming a scope
- **THEN** only that task is rated, and the open tasks below it keep whatever they
  had — including nothing

#### Scenario: Filling in the unrated ones spares the deliberate ratings

- **WHEN** the user carries a rating to the unrated tasks below
- **THEN** the tasks below with no priority take it, and a task below that already
  carries one keeps it

#### Scenario: The widest scope stops at what has ended

- **WHEN** the user carries a rating to every task below
- **THEN** every open task below it takes the new rating at any depth, and a closed
  one is left as it stands

#### Scenario: A rating below an open parent is described as ordering

- **WHEN** the user is offered the scopes for a task that is still open and has open
  tasks below it
- **THEN** they are told the tasks below come with the package and that rating them
  sets the order they are worked, not whether they are released

#### Scenario: A rating below an ended parent is described as releasing

- **WHEN** the user is offered the scopes for a task that has ended over open tasks
  below it
- **THEN** they are told those tasks will be claimed on their own and that a priority
  is what makes each claimable

#### Scenario: Both front-ends reach every scope

- **WHEN** the user sets a priority from either the CLI or the TUI
- **THEN** all three scopes are reachable from both, and both state the same effect
