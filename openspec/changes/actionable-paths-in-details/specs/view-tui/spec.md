# view-tui — delta

## ADDED Requirements

### Requirement: An agent's workspace is actionable wherever it is shown

Where a detail names an agent's workspace, it SHALL show that workspace in absolute form and
offer it as an actionable item: reachable by the detail's focus cursor, opening a shell there
when selected, and copied by the yank key. The agent's own detail SHALL name it whenever that
path resolves, independently of whether the agent has a PR open.

Where the absolute path cannot be built, the field SHALL still be shown in whatever form is
known, and SHALL NOT be offered as a location to open.

This is scoped to the agent workspace. Other details name paths — the Repos detail shows a
repo's path — and the rule is not claimed for them here.

#### Scenario: The agent's workspace is reachable

- **WHEN** an agent is selected on the Agents tab
- **THEN** its detail shows the absolute path of its workspace as an actionable item, which
  the focus cursor reaches, ENTER opens a shell in, and the yank key copies

#### Scenario: The PR's author workspace is reachable

- **WHEN** a PR is selected and its authoring agent still has a workspace
- **THEN** its detail shows that absolute path as the same kind of actionable item

#### Scenario: The path cannot be resolved

- **WHEN** the absolute path of a shown workspace cannot be determined
- **THEN** the field is still shown, and selecting it does not open a shell
