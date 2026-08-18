# 01-architecture — delta

## MODIFIED Requirements

### Requirement: Interchangeable interfaces

All UIs SHALL present the same data as similarly as possible. When one interface
shows an item's details, every other interface SHALL show the same fields for
that item.

The WORDS a state is displayed as SHALL come from the shared rendering module rather
than from the wire value, so no interface invents its own vocabulary and none drifts
from another when a word changes. A displayed word MAY differ from the stored value:
storage answers to the predicates that branch on it, display answers to the reader —
so a task the user has not ruled on stores `pending` and reads "unapproved", naming
the action that is missing rather than the state it sits in.

#### Scenario: Item detail parity

- **WHEN** the CLI and the TUI both display the same item
- **THEN** they present the same set of fields for it

#### Scenario: A state's display word changes

- **WHEN** the word a state is shown as is changed
- **THEN** it is changed once in the shared rendering module, and every interface
  shows the new word without its stored value moving
