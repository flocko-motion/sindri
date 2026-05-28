# Action: Approve

## Purpose

Defines approving a pull request, independent of any interface. Approval marks a
PR ready to merge; it is a human decision.

## Requirements

### Requirement: Approve a PR

Approving SHALL move an open PR to the approved state in the PR store. A merged
PR SHALL NOT be approvable.

#### Scenario: Approve open PR

- **WHEN** an open PR is approved
- **THEN** its status becomes approved

### Requirement: Human-only

Approving SHALL require explicit confirmation that a human is acting; agents
SHALL NOT approve their own work.

#### Scenario: Confirmation required

- **WHEN** approve is invoked
- **THEN** it confirms a human is acting before changing the PR

### Requirement: Show what is being approved

Before confirming, the action SHALL show the PR id, title, and the task summary
with its review gates, so the human approves the right thing.

#### Scenario: Pre-approval summary

- **WHEN** approve is invoked
- **THEN** the PR and its task/gate summary are shown before the confirmation
