# Work List View — delta

## MODIFIED Requirements

### Requirement: Help bar lists every list-view binding

The work-list view SHALL render a dedicated help row beneath the
title row that lists every action binding the current view accepts,
as `key:label` entries. Entries SHALL be grouped into three
` · `-separated chunks — **navigation**, **row actions**, and
**view actions** — so the eye can find an action by category. Adding
a new binding to the list view SHALL include the corresponding
help-bar entry.

For the backlog (tasks) view, the help row SHALL include every
binding the backlog accepts: cursor navigation, item open, copy,
new task, edit task, approve, reject, status, move, comment, filter
cycle, refresh, quit. The workers view SHALL drop the row-action
keys (they only apply to tasks) and keep navigation, refresh, and
quit.

#### Scenario: a:approve appears in the backlog help bar

- **GIVEN** the backlog binds `a` to "approve"
- **WHEN** the help bar renders
- **THEN** the help bar contains the entry `a:approve`

#### Scenario: x:reject appears in the backlog help bar

- **GIVEN** the backlog binds `x` to "reject task" / "abandon spec"
- **WHEN** the help bar renders
- **THEN** the help bar contains the entry `x:reject`

#### Scenario: Workers help bar drops row actions

- **WHEN** the workers view renders the help bar
- **THEN** the help bar does NOT contain row-action entries
  (a:approve, x:reject, s:status, m:move, c:comment, e:edit, n:new)
- **AND** it still shows navigation, refresh, and quit

#### Scenario: Adding a new binding without a help-bar entry is a regression

- **WHEN** the list view binds a new key K to some action
- **THEN** the help bar SHALL contain a `K:<label>` entry for it
- **AND** the regression test (golden capture of the help bar) SHALL
  refuse to pass without the entry
