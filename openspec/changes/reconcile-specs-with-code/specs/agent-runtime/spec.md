# Agent runtime — delta

## ADDED Requirements

### Requirement: An agent's memory limit is declarable and its usage observable

Each agent SHALL have a container memory limit: a per-agent value where one is set, and a
modest hub default where it is not. The limit SHALL be validated when it is set, so a
malformed size is refused at that point rather than at launch. Because a running container's
limit is fixed when the container is created, a change SHALL take effect on the agent's next
start rather than being applied to a running pod, and this SHALL be stated when the change is
made.

Usage against the limit SHALL be observable per agent — the memory in use and the limit it is
measured against, reported in the context of the runtime that enforces it, so a number is
never read without knowing what enforces it. Where usage cannot be read for one agent, that
agent's report SHALL carry the reason and SHALL NOT suppress the others.

The limit SHALL be enforced by the container runtime rather than by the hub; the hub declares
it and the runtime applies it.

#### Scenario: Setting a limit takes effect on next start

- **WHEN** the user sets an agent's memory limit while that agent is running
- **THEN** the value is stored, the user is told it applies from the agent's next start, and
  the running container is left alone

#### Scenario: A malformed size is refused

- **WHEN** the user sets a memory limit that is not a well-formed size
- **THEN** it is refused at that point, and the agent keeps its previous value

#### Scenario: Unset means the default

- **WHEN** an agent has no memory limit of its own
- **THEN** it launches with the hub's default limit

#### Scenario: Usage is reported against the limit

- **WHEN** the user asks what agents are using
- **THEN** each running agent's memory in use and its limit are reported, identified by agent
  and repo, alongside the runtime enforcing them

#### Scenario: One unreadable agent does not blank the report

- **WHEN** usage cannot be read for one agent
- **THEN** that row carries the reason and every other agent is still reported

### Requirement: Attaching reports the occupied pane, best-effort

Where an external pane tracker is present, attaching to an agent SHALL report which agent
occupies the current terminal pane, and detaching SHALL drop that claim, so a tool outside
sindri can show what is being watched where.

This SHALL be best-effort in the strict sense: a tracker that is absent, failing, or slow
SHALL NOT delay or prevent an attach or a detach, and SHALL NOT surface an error to the user
attaching. The reporting SHALL go through an adapter like any other external tool, and
deciding *when* to report SHALL belong to the interface performing the attach, not to the
adapter.

#### Scenario: Attach claims the pane

- **WHEN** a user attaches to an agent and a pane tracker is available
- **THEN** the tracker is told which agent occupies this pane

#### Scenario: Detach releases it

- **WHEN** the user detaches
- **THEN** the claim is dropped

#### Scenario: No tracker, no difference

- **WHEN** no pane tracker is installed, or reporting fails
- **THEN** the attach and detach proceed exactly as they would otherwise, with nothing
  surfaced to the user
