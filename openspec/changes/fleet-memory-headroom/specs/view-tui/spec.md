# view-tui — delta

## ADDED Requirements

### Requirement: The header carries the fleet's memory headroom

The TUI SHALL show the fleet's memory headroom in the header bar, on the right, drawn from the
figure the hub carries on the board. It SHALL read as the memory left and how many further agents
fit in it, so the question behind it — whether to start another agent — is answered at a glance from
whichever tab is open. It SHALL use the same memory meter the per-agent figures use, so one shape
means one thing across the dashboard.

The TUI SHALL NOT measure the machine itself: the figure is the hub's, read off the board.

The header SHALL yield its space to the tabs and the repo indicator. Where the terminal is too
narrow for the whole figure, the TUI SHALL show a shorter form, keeping the count of agents that fit
longest, and SHALL omit it entirely rather than push a tab off the edge.

#### Scenario: The headroom is in view from every tab

- **WHEN** the hub reports the machine's memory headroom
- **THEN** the header shows what is free and how many more agents fit, on whichever tab is open

#### Scenario: A narrow terminal keeps its tabs

- **WHEN** the terminal is too narrow for the tabs, the repo indicator and the whole figure
- **THEN** the figure is shortened, and dropped before any tab is

#### Scenario: Nothing measured, nothing drawn

- **WHEN** the board carries no memory figure
- **THEN** the header shows none, rather than a machine with nothing free
