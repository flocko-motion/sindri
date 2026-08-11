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

The same holds in the other direction. A rating on a task the approval gate still
holds releases nothing either, so every front-end SHALL make that visible where the
rating is made rather than reporting a bare success: by offering the approval, or by
naming it as what remains. Approval SHALL NOT be granted as a side effect of rating
— it decides what work exists, and a rating is not that decision. Where a rating
reached tasks below that are themselves pending, they SHALL be reported and left
pending, each verdict remaining its own act.

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

#### Scenario: Rating a task still awaiting approval offers the approval

- **WHEN** the user sets a priority on a task the approval gate still holds
- **THEN** the approval is offered in the same flow, or named as what remains, and
  the user is told the rating alone does not release the task

#### Scenario: Declining the approval is explained

- **WHEN** the user declines the approval offered after a rating
- **THEN** the task keeps the priority just set and stays in the backlog, and that
  is what the user was told would happen

#### Scenario: Rating an approved task asks nothing further

- **WHEN** the user sets a priority on a task that is already approved
- **THEN** the rating completes without offering an approval it does not need

#### Scenario: A scoped rating approves nothing it reached

- **WHEN** a rating carries to tasks below that are themselves awaiting approval
- **THEN** those tasks are reported as still awaiting it and are not approved, and
  the way to approve them is named rather than taken
