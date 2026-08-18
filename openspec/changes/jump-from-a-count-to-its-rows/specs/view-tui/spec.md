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
