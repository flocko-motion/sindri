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
editor, form, prompt or picker — the act happens on submit inside the place the key
took you to, not on the key itself. Committing is what goes behind the prefix.

A CONFIRM is not such a place, and SHALL count as part of the commit: it asks "sure?"
about an action the keystroke has already chosen, offering nothing to compose or pick.
So a key that opens a confirmation is behind the prefix, while a key that opens a form
or a picker is direct.

Whether a key commits SHALL be decided per binding and per tab, from what that
keystroke does, rather than from the letter's case: the same letter may navigate on one
tab and commit on another. Case SHALL be documented as a hint only, since uppercase
letters exist on both sides of the line.

The same key with the same label SHALL be classified the same way everywhere: it is one
action, and classifying it twice makes it inert on one tab and live on another.

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

#### Scenario: A key that opens a form

- **WHEN** the user presses a key whose action is to open a form or a picker
- **THEN** it opens directly, with no prefix, whatever case the letter has

#### Scenario: One action, one classification

- **WHEN** a key with the same label appears on more than one tab
- **THEN** it commits on all of them or none, since it is the same action

#### Scenario: A key that opens a confirmation

- **WHEN** the user presses a key whose action is to ask for confirmation of a change
- **THEN** it does nothing without the prefix: the confirm belongs to the commit
