# 05-workflow — delta

## ADDED Requirements

### Requirement: The assignment question can be asked of a role

The hub SHALL answer what would be handed out next, and why each candidate would not be, for a ROLE
as well as for an agent that exists — as a hypothetical agent of that role holding nothing. Whether
starting a reviewer would give it anything to do is otherwise answerable only by starting one and
watching.

An agent and a role together SHALL be refused rather than resolved: an agent already has a role, so
the two can contradict, and picking a winner hides the contradiction. An agent asked about by name
SHALL be answered for its own role.

Each answer SHALL come from the pool that role is actually served from, and SHALL say why each
candidate in that pool would not be offered. For a worker that is the open tasks. For a reviewer it
is the PRs, and the reasons SHALL cover the states that let one sit unreviewed while a reviewer
idled: no review was requested, one is already claimed, the PR is no longer open, or it is an
interim milestone whose merge was always the user's. A role served from no pool at all — a planner,
which is briefed, and a coauthor, which works with the user — SHALL say so in words, because an
empty list reads as "no work" rather than "not how this role gets work".

The reviewer's answer SHALL be reached under a command whose noun is the pull request, since that is
what it answers with, and both front-ends SHALL be able to ask both questions.

#### Scenario: Asking on behalf of a role nobody is running

- **WHEN** the user asks what a reviewer would be handed, with no reviewer running
- **THEN** the answer names the review that would be picked up, or says nothing is waiting

#### Scenario: An agent and a role at once

- **WHEN** the user asks about an agent and a role in the same breath
- **THEN** it is refused, rather than one of them being chosen silently

#### Scenario: Why a PR is not being reviewed

- **WHEN** an open PR would not be offered to a reviewer
- **THEN** the answer says which of the states it is in, and what would move it on

#### Scenario: A role that no pool serves

- **WHEN** the user asks what a planner or a coauthor would be handed
- **THEN** the answer says how that role gets work instead, rather than listing nothing
