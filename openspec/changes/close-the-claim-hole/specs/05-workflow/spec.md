# Workflow — delta

## ADDED Requirements

### Requirement: Every claimable task is offered by some pool

The automatic assigner SHALL offer every open, rated, ungated, unheld task through
exactly one of its pools (leaves or containers), except a task whose remaining work
is gated and waiting on an external decision. A container all of whose descendants
have closed, with no PR of its own yet merged, counts as claimable: it has nothing
left to wait on, so it SHALL be offered rather than left permanently unclaimable for
lacking an open child. Claiming such a container SHALL reuse its existing branch
rather than resetting it, since whatever its closed subtasks left there is what still
needs to reach a PR.

#### Scenario: A container with nothing left is still offered

- **WHEN** every descendant of an open, rated, unheld container has closed, and the
  container itself has no merged PR
- **THEN** the container is offered by the container pool, not excluded for having
  no open child

#### Scenario: Claiming a finished container reuses its branch

- **WHEN** an agent claims a container with nothing left to work
- **THEN** it is held on its existing branch and told to submit and finish it,
  not reset onto a fresh branch off the base

#### Scenario: A container with only gated work stays excluded

- **WHEN** a container's only remaining descendant is gated, awaiting a decision
- **THEN** it stays excluded from both pools until that gate clears, since an
  external event will release it
