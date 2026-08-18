# 05-workflow — delta

## MODIFIED Requirements

### Requirement: Plan / build / review separation

Work SHALL be separated into planning, building, and reviewing. The planner agent
SHALL shape upcoming work *with* the user — reading the repo and specs, proposing
backlog tasks, and drafting openspec — but the user SHALL retain the gates: only
the user approves a proposed task into the backlog, and only the user merges. The
worker agent SHALL build (implement tasks, open PRs); the reviewer agent SHALL
review (approve or reject the worker's PRs). A human MAY also approve or reject a
worker's PR directly from the host — review approval is not the reviewer agent's
exclusive power. Merge SHALL be human-only.

No agent SHALL approve or reject a PR built from its OWN COMMITS. The rule is about the code, not the
plan behind it: authoring a task is not authoring the work done for it, and an agent MAY rule on a PR
built from a task it wrote. That provenance SHALL be readable instead of forbidden — the verdict badge
names who ruled, and the user weighs it. The distinction is stated because the old wording ("its own
work") has been read both ways, and each reading forbids what the other allows.

#### Scenario: Roles

- **WHEN** work moves through the loop
- **THEN** the planner drafts specs and proposes tasks with the user, the user
  approves tasks and merges, the worker implements approved tasks and opens PRs,
  and the reviewer (or a human on the host) approves or rejects those PRs

#### Scenario: Human approves a worker's PR

- **WHEN** a human approves a worker's PR from the host
- **THEN** it is marked approved and may be merged, without requiring a reviewer
  agent to have approved it first

#### Scenario: Planner cannot self-serve work

- **WHEN** a planner proposes a task
- **THEN** the task is not claimable until the user approves it, so the planner
  cannot inject work into the backlog unilaterally

#### Scenario: An agent's own commits

- **WHEN** an agent approves or rejects a PR whose branch is its own work
- **THEN** the verdict is refused, and the refusal says that a PR built from a task it wrote is
  different — that one it may rule on

#### Scenario: Ruling on work built from your own plan

- **WHEN** an agent rules on a PR implementing a task that agent authored
- **THEN** the verdict is recorded, carrying who gave it, for the user to weigh

### Requirement: The coauthor works outside the managed loop

A coauthor SHALL work directly with the user, outside the managed
plan/build/review loop. It SHALL NOT claim backlog tasks, SHALL NOT open managed
PRs, and SHALL NOT pass through a review gate. It shares the user's checkout, so
the user steers and reviews its work directly in the same tree, and the coauthor
SHALL use git itself — the hub does not commit on its behalf as it does for a
worker, and there is no merge-intent to approve. This is the freestyle
counterpart to the gated worker loop; the two coexist, and the user chooses per
agent which mode they want.

A coauthor SHALL hold the strongest verb surface of any role, because it is driven directly by the
user: it reads the whole backlog, it MAY propose and revise tasks in it (`create-task`, `edit-task`),
and it MAY approve or reject another agent's PR. These verbs give reach to a human in the loop rather
than to an autonomous actor, and none of them reaches a gate the user does not still hold — a proposal
waits for the user's approval, an edit returns the task for a fresh one, and merge stays human-only.

A coauthor's approval SHALL be a full verdict rather than the planner's advisory badge. A planner's is
advisory because it rules on its own plan; a coauthor reads the PR, diffs it and runs the quality gate
against it, and what it concludes is a conclusion.

The queue SHALL never hand a coauthor a review. A reviewer PULLS work and blocks on it; a coauthor
rules when the user asks. An assigned review would leave it waiting on the fleet instead of on the
user, which is the one thing its role forbids.

A coauthor's verdict SHALL carry its own authorship, and SHALL NOT move its resting state. A reviewer's
badge lands on the review row it was assigned and returns it to its queue; a coauthor holds no such
row, so its badge is recorded under its name outright, and its rejection reaches the author in its own
voice rather than the reviewer's — an author weights feedback by who it is from.

#### Scenario: Coauthor takes no managed task

- **WHEN** a coauthor is working with the user
- **THEN** it never claims a backlog task or registers a merge-intent; the work is
  driven entirely by the user in the shared checkout

#### Scenario: Coauthor work is not gated by review

- **WHEN** a coauthor changes code in the shared checkout
- **THEN** the change is not routed through a reviewer or a merge gate; the user
  reviews it directly, since they share the tree

#### Scenario: Coauthor commits with git itself

- **WHEN** a coauthor needs to commit
- **THEN** it runs git directly in `/workspace`, rather than asking the hub to
  commit and submit a branch the way a worker does

#### Scenario: Coauthor shapes the backlog

- **WHEN** a coauthor proposes or revises a task
- **THEN** the task waits for the user's approval before any worker can claim it, exactly as a
  planner's proposal does

#### Scenario: Coauthor rules on a PR

- **WHEN** the user asks a coauthor to judge another agent's PR and it approves or rejects it
- **THEN** the verdict is recorded under the coauthor's name, its rejection reaches the author in that
  name, and the merge is still the user's

#### Scenario: No review is ever assigned to a coauthor

- **WHEN** a PR's review row is unclaimed and a coauthor is idle
- **THEN** the row stays unclaimed and the coauthor is still told to work with the user
