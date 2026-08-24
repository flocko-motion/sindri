# 05-workflow — delta

## ADDED Requirements

### Requirement: The reviewer pool spans projects

A reviewer SHALL be assignable from the virtual project `_global`, serving any project's review
queue rather than being bound to a single repo. Assigning a review to an idle reviewer SHALL
consider a `_global` reviewer alongside a project's own, preferring a reviewer local to the project
when both are idle — a repo that keeps a dedicated reviewer expects it used.

Whether a candidate reviewer already holds a review SHALL be a fleet-wide question for a `_global`
reviewer: its held review may be filed under any project it was sent to, so a project-scoped read
that finds nothing MUST NOT be read as "free" for such a reviewer.

Assigning a review to a `_global` reviewer SHALL resolve the reviewer's own roster row, workspace,
state and notes from its own project, never the PR's project. The review record itself SHALL stay
with the PR's project regardless of who is assigned to rule on it.

Every verb a reviewer runs on a PR it already holds — asking for its own directive, approving,
rejecting, showing the diff, running the lint gate against it — SHALL resolve the PR's actual
project before touching any store scoped by project, since a `_global` reviewer's own project is
never the PR's project. Operations scoped to the reviewer's own identity (its roster row, session
state, context) SHALL stay scoped to its own project throughout the same operation.

#### Scenario: A local reviewer is preferred over a pooled one

- **GIVEN** a project with its own idle reviewer and an idle `_global` reviewer
- **WHEN** an unclaimed review in that project is assigned
- **THEN** the project's own reviewer is handed it, not the pooled one

#### Scenario: A pooled reviewer serves a project with none of its own

- **GIVEN** a project with no reviewer of its own and an idle `_global` reviewer
- **WHEN** an unclaimed review in that project is assigned
- **THEN** the `_global` reviewer is handed it

#### Scenario: A pooled reviewer already busy elsewhere is not handed a second review

- **GIVEN** a `_global` reviewer already holding an unresolved review filed under a different project
- **WHEN** idle reviewers are considered for a new assignment
- **THEN** that reviewer is not offered it

#### Scenario: A pooled reviewer finds the review it holds

- **GIVEN** a `_global` reviewer holding a review filed under another project
- **WHEN** it asks for its own directive
- **THEN** it is told the PR it holds, not that nothing is pending

#### Scenario: A pooled reviewer's verdict lands on the PR's own project

- **GIVEN** a `_global` reviewer approving or rejecting a PR filed under another project
- **WHEN** it records its verdict
- **THEN** the PR's status and review row change under that PR's own project, and the reviewer's own
  session state changes under `_global`
