# view-tui — delta

## ADDED Requirements

### Requirement: Row markers come from one shared set

Every marker either front-end draws — the mark for a task being worked, its PR, a dial-in, a
warning, a retired agent, one with a clear armed, and one waiting on the user — SHALL come from a
single set both front-ends read, so that a symbol cannot come to mean one thing in the dashboard and
another in the CLI.

Each marker SHALL occupy exactly one terminal cell, and any column padded to fit markers SHALL take
its width from the markers themselves rather than from a number written beside them. Layout breaks
where the width the program computes disagrees with the width the terminal draws, and a marker
counted short pushes every column after it out of line.

The pictorial markers SHALL be Nerd Font icons from the Private Use Area, which carries no East
Asian Width for a width counter and a terminal to disagree over — the agreement holds whether or not
the reader has a patched font. A terminal without one SHALL show a box in that column and be
otherwise identical: usable, and aligned. Markers that read as well in any font — the attention mark
in the header strip among them — SHALL stay plain.

#### Scenario: A marker is added or changed

- **WHEN** a marker is introduced or redrawn
- **THEN** it is defined once in the shared set, and both front-ends show the new symbol

#### Scenario: A row carries every marker it can

- **WHEN** a task row is both worked and carries a PR
- **THEN** the marker column is exactly as wide as those marks, and the titles of every row still
  begin at the same column

#### Scenario: A terminal without the font

- **WHEN** the reader's terminal has no patched font
- **THEN** the missing icons draw as boxes in their own column and every other column stays aligned
