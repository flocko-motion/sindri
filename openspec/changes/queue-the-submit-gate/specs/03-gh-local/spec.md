# gh-local — delta

## MODIFIED Requirements

### Requirement: Lint gate before submit

Submitting (and contributing) SHALL run the project's quality gates through the same
run queue as an agent-requested run — the built-in gates (file length, dead code, and
OpenSpec validation) together with the project's own declared verify command (see
`project-config`), rather than inline. `submit`/`contribute` SHALL return at once with
the gate queued; the PR record SHALL be written only once it passes, so a failing PR is
never created. If any violation is found, or a gate cannot run (e.g. the code does not
compile), no PR SHALL be created and the violations SHALL be reported once the queued
gate completes. OpenSpec validation SHALL be skipped when the project doesn't use
openspec.

A gate run SHALL outrank an agent-requested run in the queue by default — it blocks a
PR where an exploratory run blocks nobody — and at most one gate (or exploratory run)
SHALL execute at a time across the whole fleet, so several agents submitting at once
costs one verify run at a time, not several concurrent ones.

A project that declares a verify command SHALL have it run whatever the project's
language: the absence of a Go module SHALL NOT be treated as the absence of a gate. A
project that declares none SHALL be gated by the built-in checks alone, as before.

The gate SHALL be bounded — a timeout, and output capped in a way that names what was
cut and how much, since silent truncation reads as a complete answer. Its output SHALL
be retained on the PR record and surfaced to the human, so a refused submit explains
itself without the check being re-run. A gate that does not complete within its
timeout SHALL be reported as incomplete, distinct from a lint violation: neither a
timeout nor a hub restart mid-gate found anything wrong with the code, and reporting
either as a violation would send an agent "fixing" nothing.

#### Scenario: Clean submit

- **WHEN** an agent submits work that passes every gate
- **THEN** the agent is told the gate is queued, and the PR record is created once the
  queued gate passes

#### Scenario: Lint violation

- **WHEN** an agent submits work that fails a gate (lint or an invalid spec)
- **THEN** no PR is created and the violations are shown for the agent to fix once the
  queued gate completes

#### Scenario: The project's own gate refuses a submit

- **WHEN** a project declares a verify command and it exits non-zero for an agent's
  work
- **THEN** the submit is refused, no PR is created, and the command's output is
  reported to the agent

#### Scenario: A declared gate is not skipped for a non-Go project

- **WHEN** a project with no Go module declares a verify command and an agent submits
- **THEN** the command runs and its result decides the submit, rather than the gate
  passing because no Go module was found

#### Scenario: A refused submit explains itself later

- **WHEN** a human inspects a PR whose gate failed
- **THEN** the retained gate output is shown, without the gate being run again

#### Scenario: A hanging gate does not hang the submit

- **WHEN** a project's verify command does not finish within the gate's timeout
- **THEN** the gate is reported as incomplete rather than as a lint failure, no PR is
  created, and the agent may submit again

#### Scenario: A gate outranks an exploratory run

- **WHEN** an agent-requested run is already queued and a submit's gate is queued
  behind it
- **THEN** the gate runs first, regardless of the exploratory run's priority or how
  long it has waited
