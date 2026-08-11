# gh-local — delta

## MODIFIED Requirements

### Requirement: Lint gate before submit

Submitting (and creating a PR) SHALL run the project's quality gates before the PR record is
written — the same gates as `sindri lint all` (file length, dead code, and OpenSpec validation). If
any violation is found, or a gate cannot run (e.g. the code does not compile), the submit SHALL be
refused and the violations reported, so a failing PR is never created. OpenSpec validation SHALL be
skipped when the project doesn't use openspec.

The gates SHALL run on a branch that is current with its base, and a submit whose branch the base
has moved past SHALL be refused before they run. The hub SHALL NOT bring the branch up to base on
the agent's behalf here: the gate result is attached to the PR, so a rebase performed after it would
record a verdict taken against the old base while the state that actually merges was never gated.
The refusal SHALL name how far behind the branch is and the operation that brings it current, and
the agent SHALL reach a PR by rebasing and submitting again — which runs the gates on the tree that
will land.

A refused submit SHALL leave the agent's work as it found it, committing nothing and recording no
PR, so a second attempt is the same submission rather than one layered on the last.

#### Scenario: Clean submit

- **WHEN** an agent submits work that passes every gate
- **THEN** the PR record is created

#### Scenario: Lint violation

- **WHEN** an agent submits work that fails a gate (lint or an invalid spec)
- **THEN** no PR is created and the violations are shown for the agent to fix

#### Scenario: Submitting from a branch the base has moved past

- **WHEN** an agent submits a branch that is behind its base
- **THEN** no PR is created, the agent is told how far behind it is and what arrived, and is
  directed to rebase and submit again

#### Scenario: The gate does not run on a stale branch

- **WHEN** a submit is refused for being behind its base
- **THEN** the quality gates are not run for that attempt, since their result would describe a tree
  that is not the one to be merged

#### Scenario: Submitting after rebasing

- **WHEN** an agent rebases and submits again
- **THEN** the gates run on the rebased tree and the PR is created against the current base
