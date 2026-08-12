# gh-local — delta

## MODIFIED Requirements

### Requirement: Lint gate before submit

Submitting (and creating a PR) SHALL run the project's quality gates after the
rebase and before the PR record is written — the built-in gates (file length, dead
code, and OpenSpec validation) together with the project's own declared verify command
(see `project-config`). If any violation is found, or a gate cannot run (e.g. the code
does not compile), the submit SHALL be refused and the violations reported, so a
failing PR is never created. OpenSpec validation SHALL be skipped when the project
doesn't use openspec.

A project that declares a verify command SHALL have it run whatever the project's
language: the absence of a Go module SHALL NOT be treated as the absence of a gate. A
project that declares none SHALL be gated by the built-in checks alone, as before.

The gate SHALL be bounded — a timeout, and output capped in a way that names what was
cut and how much, since silent truncation reads as a complete answer. Its output SHALL
be retained on the PR record and surfaced to the human, so a refused submit explains
itself without the check being re-run.

#### Scenario: Clean submit

- **WHEN** an agent submits work that passes every gate
- **THEN** the PR record is created

#### Scenario: Lint violation

- **WHEN** an agent submits work that fails a gate (lint or an invalid spec)
- **THEN** no PR is created and the violations are shown for the agent to fix

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
- **THEN** the gate reports the timeout and refuses the submit, rather than blocking
  the agent indefinitely
