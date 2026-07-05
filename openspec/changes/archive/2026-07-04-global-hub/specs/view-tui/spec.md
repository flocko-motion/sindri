## MODIFIED Requirements

### Requirement: Tabs are sections with live actionable counts

The TUI SHALL present tabs from the hub's section model, each titled `[<n> <Title>]`
where `<n>` is the section's actionable count. The Agents and PRs tabs SHALL be
global across all projects — each row identifying its repo — so counts reflect
running agents and not-merged PRs everywhere. The Tasks tab SHALL be scoped to the
currently selected repo. The counts SHALL update live as board state changes; the
TUI SHALL NOT compute them itself.

#### Scenario: Count reflects state across repos

- **WHEN** an agent starts running or a PR is merged in any repo
- **THEN** the corresponding global tab badge updates without a manual refresh

#### Scenario: Tasks follow the selected repo

- **WHEN** the user switches the selected repo
- **THEN** the Tasks tab shows that repo's tasks, while the Agents and PRs tabs
  stay global

### Requirement: The TUI is a hub client

The TUI SHALL get all data from the single global hub (`/state` + `/events`) and
perform all mutations through it, holding no domain logic of its own. When no hub is
running it SHALL auto-start a background hub rather than refusing.

#### Scenario: No hub yet

- **WHEN** the TUI starts and no hub is running
- **THEN** it starts a background hub, then connects

## ADDED Requirements

### Requirement: Repo switcher scopes the per-repo view

The TUI SHALL provide a repo switcher, presented as a picker overlay listing the
projects the hub knows (from the project registry). Selecting a repo SHALL scope the
Tasks tab (and any other per-repo view) to it, without affecting the global Agents
and PRs tabs.

#### Scenario: Switching repos

- **WHEN** the user opens the switcher overlay and picks a repo
- **THEN** the per-repo view rescopes to it and the global tabs are unchanged

#### Scenario: Rows carry their repo

- **WHEN** the Agents or PRs tab lists entries from more than one repo
- **THEN** each row shows which repo it belongs to

### Requirement: Each project has a deterministic color scheme

The TUI SHALL give each project a color scheme derived deterministically from its
stable key (`repoTag`), so the same repo always renders in the same colors across
sessions. A scheme SHALL be a *(primary, accent)* pair selected from a fixed palette
by hashing the project key, giving a space of `primary × accent` combinations large
enough that the handful of repos in use rarely collide. The current project's scheme
SHALL tint the board chrome (the active-repo indicator / header), and per-row repo
tags SHALL carry their project's color, so entries from different repos are visually
separable.

#### Scenario: Stable color per repo

- **WHEN** the same repo is shown in different sessions
- **THEN** it renders with the same color scheme both times

#### Scenario: Repos are visually distinguishable

- **WHEN** the Agents or PRs tab shows rows from several repos
- **THEN** each repo's rows carry its own scheme's color, and the selected repo tints
  the board chrome
