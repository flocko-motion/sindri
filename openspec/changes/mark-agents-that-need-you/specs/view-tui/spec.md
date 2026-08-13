# view-tui — delta

## MODIFIED Requirements

### Requirement: Tabs are sections with live actionable counts

The TUI SHALL present tabs from the hub's section model, each titled `[<n> <Title>]`
where `<n>` is the section's actionable count. The Agents and PRs tabs SHALL be
global across all projects — each row identifying its repo — so counts reflect
running agents and not-merged PRs everywhere. The Tasks tab SHALL be scoped to the
currently selected repo. The counts SHALL update live as board state changes; the
TUI SHALL NOT compute them itself.

A tab whose section has rows waiting on the user SHALL append `(<n>!)` to its title,
where `<n>` is the section's attention count. The marker SHALL be drawn the same way
for every section, from the count the hub resolved, so the TUI decides which rows
qualify for none of them. It SHALL be absent at zero: it is a call to act, not
furniture. It rides on the handle so it is legible from whichever tab is open —
"why is nothing happening?" is rarely asked from the tab holding the answer.

#### Scenario: Count reflects state across repos

- **WHEN** an agent starts running or a PR is merged in any repo
- **THEN** the corresponding global tab badge updates without a manual refresh

#### Scenario: Tasks follow the selected repo

- **WHEN** the user switches the selected repo
- **THEN** the Tasks tab shows that repo's tasks, while the Agents and PRs tabs
  stay global

#### Scenario: A section is waiting on the user

- **WHEN** the hub reports an attention count above zero for a section
- **THEN** that tab's title carries `(<n>!)` beside its badge, visible from every tab

#### Scenario: Nothing is waiting

- **WHEN** a section's attention count is zero
- **THEN** its title carries no marker
