# view-tui — delta

## ADDED Requirements

### Requirement: The Runs tab says what a run is, permanently

The Runs tab SHALL carry an explanatory line at the top at ALL times, not only when the list is
empty. It is the least self-evident of the tabs: nothing else on it says what a run is, and the
surprising part — one run at a time across every repo — is what "why has mine not started" asks, a
question that arrives when the list is FULL. A line that disappeared as soon as there were rows
would hide itself exactly when it starts being useful.

The line SHALL say what a run IS — a command executed against a workspace, one at a time across
every repo — and who can queue one: the user, and agents.

It SHALL occupy one line where there is room and SHALL wrap rather than truncate, to no more than
two. A half-sentence explains nothing. Its lines SHALL come out of the rows' space rather than
pushing rows off the screen, and the row area SHALL never collapse to nothing however short the
terminal.

The empty state SHALL remain, distinct from the line and where the rows would be. The line says
what runs are; it does not say whether there are any, and a tab with neither reads as one that
failed to load.

#### Scenario: The line is there whether or not runs are

- **WHEN** the Runs tab is shown with no runs, and again with several
- **THEN** the same explanatory line is present both times

#### Scenario: A narrow terminal wraps it

- **WHEN** the terminal is too narrow for the line
- **THEN** it wraps to a second line rather than truncating, and never to a third

#### Scenario: The rows survive the line

- **WHEN** the terminal is short
- **THEN** the line takes lines from the list rather than pushing the runs out of view

#### Scenario: An empty list says so

- **WHEN** the Runs tab has no runs
- **THEN** the row area says so, beneath the explanatory line
