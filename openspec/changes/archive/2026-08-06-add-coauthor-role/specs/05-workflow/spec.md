# Workflow — delta

## ADDED Requirements

### Requirement: The coauthor works outside the managed loop

A coauthor SHALL work directly with the user, outside the managed
plan/build/review loop. It SHALL NOT claim backlog tasks, SHALL NOT open managed
PRs, and SHALL NOT pass through a review gate. It shares the user's checkout, so
the user steers and reviews its work directly in the same tree, and the coauthor
SHALL use git itself — the hub does not commit on its behalf as it does for a
worker, and there is no merge-intent to approve. This is the freestyle
counterpart to the gated worker loop; the two coexist, and the user chooses per
agent which mode they want.

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
