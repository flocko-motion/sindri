# gh-local — delta

## MODIFIED Requirements

### Requirement: Lint gate before submit

Submitting (and contributing) SHALL run the project's quality gate through the run queue, not
inline: `submit`/`contribute` SHALL return at once with the gate queued, and the PR record SHALL be
written only once it passes, so a failing PR is never created. If any violation is found, or a gate
cannot run (e.g. the code does not compile), no PR SHALL be created and the violations SHALL be
reported once the queued gate completes. OpenSpec validation SHALL be skipped when the project
doesn't use openspec.

The gate SHALL check a COMMIT, never "whatever is on disk". The hub SHALL record the agent's
workspace as a commit before the gate opens — agents have no commit verb of their own — and the
gate's verdict SHALL be recorded against that commit's sha, since a timestamp cannot say which tree
was checked.

It SHALL measure a fresh CHECKOUT of that commit, not the worktree the commit came from. A
self-check parks nobody — the agent is told to carry on with other work while it waits — so gating
its live tree would build and test a moving target, report violations about a file the agent never
asked about, and file the result under a sha it was never taken on. Where there is no commit to check
out (a tree the hub does not own), the tree SHALL be gated as it stands and NO verdict recorded.

EVERY invocation of the gate SHALL go through the same queue: the two landing verbs, an agent's own
self-check, a human's or reviewer's check of a PR, and the periodic re-check of an open PR against a
moved base. At most one SHALL execute at a time across the whole fleet, so several agents checking
their work at once costs one gate at a time rather than several concurrent builds and test suites on
one host. A gate that somebody is held up by SHALL outrank one nobody is: the landing verbs and a
self-check before an agent-requested run, and the periodic re-check as an ordinary run.

A request for a check already queued SHALL join it rather than queue a second, and EVERY agent that
asked SHALL be told when it lands. An agent cannot watch the board a human reads, so one left waiting
on a message nobody sends is parked with nothing coming — while holding a verdict it was told to have
before deciding.

A project that declares a verify command SHALL have it run whatever the project's language: the
absence of a Go module SHALL NOT be treated as the absence of a gate. Exactly ONE of the two checks
SHALL run — the declared command, or the built-in checks when none is declared — never both (see
`project-config`).

A check of a PR SHALL be against what the PR contains — the commit its branch names — and not
against its author's working tree, which after a submit may hold work no commit on the branch
carries.

The gate SHALL be bounded — a timeout, and output capped in a way that names what was cut and how
much, since silent truncation reads as a complete answer. Its output SHALL be retained against the
PR and the commit, and surfaced to the human, so a refused submit explains itself without the check
being re-run. A gate that does not complete within its timeout SHALL be reported as incomplete,
distinct from a lint violation: neither a timeout nor a hub restart mid-gate found anything wrong
with the code, and reporting either as a violation would send an agent "fixing" nothing.

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

#### Scenario: An advisory re-check waits behind work someone is held up by

- **WHEN** the periodic re-check of an open PR is queued and an agent then submits
- **THEN** the agent's gate runs first — it is parked until it answers, while nobody is
  waiting on the re-check

#### Scenario: A self-check is queued, not run in the hub

- **WHEN** an agent asks for the gate on its own workspace
- **THEN** it is told at once where the check sits in the queue, and the result reaches
  it as a message rather than as the reply

#### Scenario: An agent editing during its own check does not change its verdict

- **GIVEN** an agent whose self-check is queued, which then edits its workspace as it was told it may
- **WHEN** the check runs
- **THEN** it measures the commit it was queued for, the verdict is filed under that commit, and the
  agent's edit is neither checked nor disturbed

#### Scenario: A second asker joins the queued check and is still told

- **GIVEN** a check of a PR already queued because a human asked for it
- **WHEN** a reviewer asks for the same PR's verdict
- **THEN** no second check is queued, and the reviewer is told when the queued one lands

#### Scenario: A PR check is against the PR

- **GIVEN** an agent whose PR is under review and whose worktree holds later, uncommitted work
- **WHEN** the gate is asked about that PR
- **THEN** it checks the commit the PR's branch names, and the author's worktree is left
  exactly as it was

## ADDED Requirements

### Requirement: A gate verdict is reusable because it names a commit

A gate verdict SHALL be stored against the commit it describes, together with the verify command
that produced it, and a request to gate a commit with a stored PASS under the same command SHALL be
answered from that record rather than by running the gate again. This is what makes the ordinary
sequence — check, change nothing, submit — cost one gate rather than two.

A reused answer SHALL say that it was reused and name the commit. A result that reads like a fresh
one hides the fact that nothing ran, which is worse than the cost it saved.

A stored FAILURE SHALL NOT decide anything. A gate can fail for reasons outside the tree — a flaky
test, a host under load — and a commit pinned to one bad run would need a human to unpin it. It SHALL
nonetheless be READABLE: a reader asking what a check said about a commit SHALL be answered from the
record, pass or fail, because one that has to re-run the gate to see why it failed pays for a whole
build and test — in the fleet's single slot — every time it looks. Reading is not deciding.

A verdict SHALL NOT be reused when the project's declared gate has changed, since a different
command asked a different question of the same tree.

#### Scenario: An unchanged workspace is not gated twice

- **GIVEN** an agent whose self-check passed and who has changed nothing since
- **WHEN** it submits
- **THEN** the stored pass is reused, no gate runs, and the PR is created

#### Scenario: A reused answer says so

- **WHEN** a gate result is reused
- **THEN** the answer names the commit it describes and states that nothing was re-run

#### Scenario: A changed workspace is gated again

- **GIVEN** an agent whose self-check passed and which has since edited a file
- **WHEN** it submits
- **THEN** a gate is queued for the new commit, and no PR exists until it passes

#### Scenario: A failure decides nothing but reads back

- **GIVEN** a PR whose gate failed at its branch's current commit
- **WHEN** a reviewer asks what the gate said
- **THEN** the recorded failure is returned, marked as recorded rather than fresh, and no gate is
  queued; a submit of that same commit is still gated again rather than refused from the record

#### Scenario: Re-pointing the project's gate invalidates its passes

- **GIVEN** a commit with a stored pass under one declared verify command
- **WHEN** the project declares a different one and the commit is gated
- **THEN** the gate runs, because the stored answer was to a different question
