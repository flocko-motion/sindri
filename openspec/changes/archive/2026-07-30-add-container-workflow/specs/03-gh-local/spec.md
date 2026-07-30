# gh-local — delta

## ADDED Requirements

### Requirement: Container branches persist across subtasks

A held container's worktree branch SHALL be named for the container and persist
across all its subtasks, decoupled from the agent's current subtask: completing
one subtask and starting the next SHALL land both as commits on the one container
branch and SHALL NOT create or rename a branch. (This is the collaborative
exception to one-branch-per-leaf; a structured leaf task still gets its own
branch.)

#### Scenario: One branch, many subtasks

- **WHEN** an agent completes a subtask and moves to the next within the same
  container
- **THEN** both land as commits on the single container branch, which is neither
  recreated nor renamed between them

#### Scenario: Branch outlives the current subtask

- **WHEN** the agent's current subtask changes
- **THEN** the branch name does not, because it tracks the container, not the
  subtask

### Requirement: Milestone merge does not retire the branch

A container branch MAY be merged at a milestone: the merge SHALL land the branch's
current state into the base, rebase the branch onto the new base, and leave the
branch in place for continued work — distinct from a terminal merge, which retires
the branch and frees the agent. A milestone merge stays human-only and gated on an
approved PR (the human may approve it directly). The branch is retired only when
its container is closed.

#### Scenario: Milestone merge keeps the branch

- **WHEN** a container branch is merged at a milestone
- **THEN** its current state lands on base, the branch is rebased onto the new
  base, and it remains checked out for the agent to keep working

#### Scenario: Terminal merge on container completion

- **WHEN** a container is closed (all its children done) and its branch is merged
- **THEN** the branch is retired and the agent is freed to take new work
