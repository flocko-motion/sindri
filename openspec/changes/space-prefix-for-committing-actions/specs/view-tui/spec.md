# view-tui — delta

## ADDED Requirements

### Requirement: Committing actions live behind a prefix key

Every key that CHANGES STATE SHALL be reachable only through a prefix. Pressing the
prefix SHALL open a menu that both shows the committing actions available and makes
their letters live; pressing one of those letters without the menu open SHALL do
nothing. Escape, or the prefix again, SHALL close the menu having changed nothing, and
a letter the menu did not offer SHALL do nothing but close it.

The letters SHALL NOT change when an action moves behind the prefix: what is learned is
one deliberate keystroke in front, not a new vocabulary.

The menu SHALL be generated from the same table that declares the bindings and renders
the footers, so the dispatcher, the menu and the help cannot describe different worlds.

The menu SHALL offer only what applies to the selected row — no merge on an unapproved
PR, no unassign on a task nobody holds — so it answers "what can I do with this" rather
than "what exists". Whether a key is inert outside the menu SHALL NOT depend on the
selection: a key that commits anywhere on the tab is never live bare, or whether a stray
press did something would depend on which row happened to be selected.

The convention SHALL read NAVIGATE OR COMMIT, and SHALL carry no exception clause.
Navigating is direct: moving the selection, changing what is shown, and opening an
editor, form, prompt or chooser — the act happens on submit inside the place the key
took you to, not on the key itself. Committing is what goes behind the prefix.

The footer SHALL advertise the navigating keys and one entry for the prefix, naming it
readably rather than as a blank, and SHALL say when the selected row offers nothing.

#### Scenario: A committing key on its own

- **WHEN** the user presses a committing letter without the prefix
- **THEN** nothing happens: no action, no prompt, no modal

#### Scenario: The prefix then the letter

- **WHEN** the user presses the prefix and then that letter
- **THEN** the action runs exactly as it did when the letter was direct

#### Scenario: Leaving the menu

- **WHEN** the user presses escape, the prefix again, or a letter the menu did not offer
- **THEN** the menu closes and nothing has changed

#### Scenario: The menu reflects the row

- **WHEN** the selected PR is not approved
- **THEN** merge is absent from the menu, and pressing it after the prefix does nothing

#### Scenario: Navigation is unaffected

- **WHEN** the user presses a navigating key
- **THEN** it acts directly, with no prefix, as it always has
