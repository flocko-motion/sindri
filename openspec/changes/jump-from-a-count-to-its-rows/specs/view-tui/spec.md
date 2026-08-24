# view-tui — delta

## ADDED Requirements

### Requirement: A count in a detail pane leads to the rows behind it

A count shown in a detail pane SHALL be selectable, and choosing it SHALL open the tab holding the
rows it counts, narrowed to exactly that set. A number alone is the beginning of a question — which
ones, from whom, saying what — and a pane that states it without a way through leaves the question
unanswered.

The destination SHALL be narrowed on every axis the count itself was counted on. Landing on a
broader set than the number promised is a view contradicting itself within two keystrokes, and is
worse than not jumping at all.

Where the counted rows lie outside the view's current repo scope, the jump SHALL widen the scope
rather than land on an empty list: the count belongs to the thing it was shown on, so an empty
destination reads as "there is none" when the truth is "you are looking at the wrong repo". A
widening SHALL be stated, since it changes a setting the user did not touch.

#### Scenario: An agent's unread mail is opened from its detail

- **WHEN** the user selects the unread-mail count on an agent's detail
- **THEN** the Mail tab opens, narrowed to that agent and to unread, showing exactly as many
  messages as the count named

#### Scenario: The counted rows are in another repo

- **WHEN** the count belongs to an agent outside the repo currently in scope
- **THEN** the destination still shows that agent's rows, and the change of scope is stated

#### Scenario: An agent with nothing unread

- **WHEN** an agent has no unread mail
- **THEN** no count is shown, and nothing new is selectable on its detail

### Requirement: A narrowed list says how it is narrowed

Where a list is narrowed beyond the view a tab opens with, it SHALL say so in a line above the rows,
naming every axis in force — the status filter, any recipient it is narrowed to, and the repo scope
— and naming the key that clears them. Four rows cannot otherwise be told from a slice of forty, and
a narrowing arrived at by navigation rather than set on purpose is the one a user is least prepared
for.

The line SHALL appear ONLY when something is non-default. The ordinary view SHALL look exactly as it
did: a permanent banner spends a row of screen explaining the usual case, and an eye stops reading a
line that is always there.

When shown, it SHALL name every axis in force, defaults included, because the axis that raised the
line is rarely the only one hiding rows. It SHALL be a line the cursor cannot rest on, by the same
mechanism as the column labels and the section headings, rather than a second way of putting an
unselectable line in a list.

One key SHALL clear every axis at once, restoring what the tab opens with rather than the widest
possible view — widening to "show everything" would leave a filter the user must clear in turn, and
the line on screen. On a view that is not narrowed that key SHALL do nothing.

#### Scenario: A list narrowed by a jump from another tab

- **WHEN** a jump narrows a list to a set the user did not configure
- **THEN** a line above the rows names each axis in force and the key that clears them

#### Scenario: The ordinary view

- **WHEN** every axis is at the value the tab opens with
- **THEN** no such line is shown

#### Scenario: A narrowing that hides everything

- **WHEN** the axes in force admit no rows at all
- **THEN** the line is still shown, so an emptied list is not read as an empty one

#### Scenario: Clearing

- **WHEN** the user presses the clearing key on a narrowed list
- **THEN** every axis returns to the tab's default, and the line goes
