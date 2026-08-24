# view-workers — delta

## ADDED Requirements

### Requirement: Every CLI listing names its columns

Every CLI listing SHALL print a line of column labels above its rows — `sindri agent list`,
`agent stats`, `task list`, `pr list`, `run list`, `mail list` and `repo list`. It is the same
confusion as the TUI's and the same fix: a reader decodes an unlabelled column from its values, and
two adjacent columns of the same shape leave nothing to decode against.

The labels SHALL be laid out by the SAME column widths as the rows, from one layout producing both.
Widths written twice agree on the day they are written and drift afterwards, and a header that no
longer sits over its columns is worse than no header.

No label line SHALL be printed where there are no rows: each command's own closing line already says
why a listing is empty, and labels over nothing explain a table that is not there.

Where a label names the column, a row SHALL NOT repeat it. A count printed as "3 agents" on every line
was saying once per row what the header says once.

#### Scenario: Listing with rows

- **WHEN** any of those commands prints rows
- **THEN** a label line precedes them, each label over the column it names

#### Scenario: Listing with none

- **WHEN** the command has no rows to print
- **THEN** no label line appears, and its closing line says why

### Requirement: `mail list` reads sender before recipient

`sindri mail list` SHALL print the sender before the recipient. The two are adjacent columns of
agent-shaped names, so nothing in the row corrects a misreading, and `to` before `from` is the reverse
of how mail is read everywhere else.

#### Scenario: A listed message

- **WHEN** `sindri mail list` prints a message
- **THEN** its sender column precedes its recipient column, each labelled
