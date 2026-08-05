# Hub — delta

## MODIFIED Requirements

### Requirement: Hub lifecycle — one persistent global daemon

The hub SHALL be a single long-lived daemon serving all repos, running as its own
program rather than inside a front-end's process. Interactive entry points (`sindri
coauthor`, `sindri tui`) SHALL auto-start it in the background when none is running,
by executing the hub binary; `sindri hub start` runs it explicitly (foreground, or
`--bg`) and SHALL likewise hand off to that binary rather than constructing the hub
in the calling process. The hub binary SHALL be resolved beside the running
front-end before any search of `PATH`, so a second installed copy cannot be started
by accident. Once running it SHALL persist across individual CLI commands and for as
long as any agent in any repo exists. When the hub is not running, an agent's call
SHALL fail loudly.

#### Scenario: Interactive command with no hub

- **WHEN** a user runs `sindri coauthor` or `sindri tui` and no hub is running
- **THEN** a background hub is started, then the command proceeds

#### Scenario: Agents keep the hub alive

- **WHEN** agents are running in any repo
- **THEN** the single hub persists rather than exiting

#### Scenario: Foreground start hands off to the hub binary

- **WHEN** a user runs `sindri hub start` in the foreground
- **THEN** the hub binary takes over the process, so signals reach the hub directly
  with no wrapper process between the terminal and the daemon

#### Scenario: The hub beside this build is the one that starts

- **WHEN** the hub binary is resolved for a start
- **THEN** it is taken from beside the running front-end binary before `PATH` is
  consulted, so a second installed copy is never started in its place

### Requirement: Sections with actionable counts

The hub SHALL expose a section model — an ordered set of sections, each with a
key, a title, and a count derived from board state — as the single source of
truth for which views exist and the badge each shows. The counts SHALL be the
actionable subset: non-closed tasks, running agents, and not-merged PRs. UIs
SHALL render these counts rather than computing their own.

What crosses to a client SHALL be the **resolved** section: a key, a title and a
number. The recipe that derives a count from board state SHALL stay inside the hub,
because a function cannot cross the boundary; the hub SHALL resolve every count
against the board it is already serving.

#### Scenario: A UI renders section counts

- **WHEN** a UI draws its tabs
- **THEN** each tab's badge is the hub-provided count for that section

#### Scenario: Adding a section

- **WHEN** a new section is introduced
- **THEN** it is added to the hub's section model and UIs pick it up without
  re-deriving counts

#### Scenario: Counts arrive resolved

- **WHEN** the board is served
- **THEN** each section carries its key, title and computed count, and no part of the
  derivation crosses to the client

### Requirement: Task hierarchy arrangement

A flat set of tasks SHALL be arrangeable into their parent/child tree — roots
ordered by priority, each followed by its descendants, with a depth per node —
and annotated with the id of a non-merged PR for that task, if any. A task
whose parent is absent from the set SHALL be arranged as a root.

This arrangement SHALL be a function of the exchange format, so the hub and every
front-end obtain the same tree from the same code, and no interface derives its own.
The arrangement SHALL carry data rather than drawing instructions: depth, the PR id
and its kind are data; a hint that exists only to draw a tree connector belongs to
the interface drawing it.

#### Scenario: Tree with depth

- **WHEN** tasks with parent relationships are arranged
- **THEN** the result lists each parent before its children with an increasing
  depth, and standalone tasks at depth zero

#### Scenario: PR annotation

- **WHEN** a task has a non-merged PR
- **THEN** its arranged row carries that PR's id

#### Scenario: One arrangement, every caller

- **WHEN** the hub and a front-end both arrange the same task set
- **THEN** both call the same function from the exchange package and produce the
  same tree
