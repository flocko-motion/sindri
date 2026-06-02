# Work List View — delta

## ADDED Requirements

### Requirement: Help bar lists every list-view binding

The work-list view SHALL render every action binding the list view
accepts as a `key:label` entry in the top help bar, so the help bar is
the visible inventory of what the user can do. Adding a new binding to
the list view SHALL include the corresponding help-bar entry.

#### Scenario: e:edit appears in the help bar

- **GIVEN** the list view binds `e` to "edit task"
- **WHEN** the help bar renders
- **THEN** the help bar contains the entry `e:edit`

#### Scenario: Adding a new binding without a help-bar entry is a regression

- **WHEN** the list view binds a new key K to some action
- **THEN** the help bar SHALL contain a `K:<label>` entry for it
- **AND** the regression test (golden capture of the help bar) SHALL
  refuse to pass without the entry
