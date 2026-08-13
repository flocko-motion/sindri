# 05-workflow — delta

## ADDED Requirements

### Requirement: A PR waiting on the human is marked

A PR SHALL count as waiting on the user when nothing but a human will move it on.
Three states qualify, each following from a gate the workflow already has:

- **approved and unmerged** — merge is human-only and the one hard gate, so an
  approved PR is finished work waiting on a person to land it
- **open and interim** — a mid-task contribution or a milestone is user-gated by
  design and no reviewer is ever asked for one, so it is the user's from the moment
  it opens; a milestone blocks its agent until the merge lands
- **open with no reviewer alive** — no review is coming, so the user must review it
  themselves or start a reviewer

Whether a reviewer is alive SHALL be judged across the fleet rather than per PR: an
open PR with nobody assigned is ordinary while a reviewer is running, since one picks
it up shortly, and it is the absence of every live reviewer that strands the queue. A
rejected PR SHALL NOT count — it waits on its author to resubmit.

This count SHALL be the PRs section's attention count, derived in the exchange package
and resolved by the hub with the others, so every interface renders one number rather
than deciding for itself which PRs qualify. Every front-end SHALL surface it: the TUI
as the marker on its PRs handle, the CLI in its PR listing, naming the PRs and the
action that clears each — an approved PR and a stranded one are cleared by different
commands.

#### Scenario: Approved work nobody has landed

- **WHEN** a PR is approved and not yet merged
- **THEN** it is counted as waiting on the user, whatever agents are running

#### Scenario: No reviewer to review it

- **WHEN** a PR is open and no reviewer agent is running anywhere
- **THEN** it is counted as waiting on the user, and the listing says a review will
  not arrive on its own

#### Scenario: A reviewer is running

- **WHEN** a PR is open, unassigned, not interim, and a reviewer agent is running
- **THEN** it is not counted: the queue is moving

#### Scenario: An interim PR with reviewers running

- **WHEN** an interim PR is open while reviewer agents are running
- **THEN** it is still counted, and the listing says a reviewer will not look at it —
  starting another would change nothing

#### Scenario: A rejected PR waits on its author

- **WHEN** a PR has been rejected
- **THEN** it is not counted as waiting on the user
