# Work List View — delta

## ADDED Requirements

### Requirement: Help-bar hints SHALL be grouped by purpose

The list-view help bar SHALL group its key hints into three " · "-separated
sections: navigation hints (cursor + open), row-scoped actions (copy, new),
and view-scoped actions (refresh, quit). The binding strings themselves
SHALL remain unchanged.

#### Scenario: Default list view

- **WHEN** the list view renders the help bar
- **THEN** it reads "j/k:nav enter:open · y:copy n:new · r:refresh q:quit"

#### Scenario: Workers view

- **WHEN** the workers view renders the help bar
- **THEN** the same three-group structure is used
