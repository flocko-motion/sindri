# 05-workflow — delta

## ADDED Requirements

### Requirement: A PR waiting on the human is marked

A PR SHALL count as waiting on the user when nothing but a human will move it on.
Four states qualify, each following from a gate the workflow already has:

- **merge-failed** — a hub restart caught the merge in flight, so whether the base
  branch carries it is unknown. Nothing retries it and no verb accepts it, so it moves
  only when a person inspects the base; it is the most stuck state there is
- **approved and unmerged** — merge is human-only and the one hard gate, so an
  approved PR is finished work waiting on a person to land it
- **open and interim** — a mid-task contribution or a milestone is user-gated by
  design and no reviewer is ever asked for one, so it is the user's from the moment
  it opens; a milestone blocks its agent until the merge lands
- **open with no reviewer alive in its repo** — no review is coming, so the user must
  review it themselves or start a reviewer

Whether a reviewer is alive SHALL be judged PER REPO, because review assignment is: a
reviewer is chosen from its own project's roster, so one running elsewhere is never
handed this PR and its being up says nothing about whether a review will arrive. Within
that repo the question is liveness rather than assignment — an open PR with nobody
assigned is ordinary while a reviewer runs there, since one picks it up shortly. A
rejected PR SHALL NOT count: it waits on its author to resubmit. Neither SHALL a PR
whose merge is under way: it waits on that merge.

This count SHALL be the PRs section's attention count, derived in the exchange package
and resolved by the hub with the others, so every interface renders one number rather
than deciding for itself which PRs qualify. WHICH of the states a PR is in SHALL come
from that same place, as a value a front-end renders: each is cleared by a different
command, so a front-end that re-derived the classification could name the wrong one
with confidence. Every front-end SHALL surface it — the TUI as the marker on its PRs
handle, the CLI in its PR listing, naming the PRs and the action that clears each.

#### Scenario: A merge orphaned by a restart

- **WHEN** a hub restart leaves a PR merge-failed
- **THEN** it is counted as waiting on the user, and the listing sends them to the base
  branch rather than to any verb — none will take it

#### Scenario: Approved work nobody has landed

- **WHEN** a PR is approved and not yet merged
- **THEN** it is counted as waiting on the user, whatever agents are running

#### Scenario: No reviewer to review it

- **WHEN** a PR is open and no reviewer agent is running in its repo
- **THEN** it is counted as waiting on the user, and the listing says a review will
  not arrive on its own

#### Scenario: A reviewer in another repo

- **WHEN** a PR is open in a repo with no reviewer, while another repo runs one
- **THEN** it is still counted: a reviewer is only ever handed its own repo's PRs

#### Scenario: A reviewer is running

- **WHEN** a PR is open, unassigned, not interim, and a reviewer agent is running in its repo
- **THEN** it is not counted: the queue is moving

#### Scenario: An interim PR with reviewers running

- **WHEN** an interim PR is open while reviewer agents are running
- **THEN** it is still counted, and the listing says a reviewer will not look at it —
  starting another would change nothing

#### Scenario: A rejected PR waits on its author

- **WHEN** a PR has been rejected
- **THEN** it is not counted as waiting on the user
