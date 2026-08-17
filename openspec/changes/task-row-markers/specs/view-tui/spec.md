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

### Requirement: A task names the agent behind it

A task's detail SHALL name the agent behind it, in both front-ends, and SHALL keep naming one while
its pull request waits on a verdict — a submitted task still has an owner, and who submitted it is
what a reader opens it to find out.

The name and the row's worked-on marker SHALL come from one rule, so a marked row always has a name
behind it: the agent holding the task, the agent holding it as a feature container, and failing
either, the author of its open PR. A live claim SHALL outrank a PR, since an agent put back on a
rejected task is working it again while the PR it was rejected from is still on the board.

The row itself SHALL carry the marker rather than the name: the marker column is padded to keep
every title starting at the same place, and a name is as long as it happens to be.

#### Scenario: A task under review

- **WHEN** a task's PR is waiting on a verdict and no agent holds the task
- **THEN** its detail names the agent that submitted it

#### Scenario: The row and the detail agree

- **WHEN** a row carries the worked-on marker
- **THEN** that task's detail names an agent, and the same one in either front-end

#### Scenario: Nobody is working it

- **WHEN** no agent holds a task and it has no open PR
- **THEN** the detail shows the field as empty rather than naming a past owner
