# hub — delta

## MODIFIED Requirements

### Requirement: Sections with actionable counts

The hub SHALL expose a section model — an ordered set of sections, each with a
key, a title, and a count derived from board state — as the single source of
truth for which views exist and the badge each shows. The counts SHALL be the
actionable subset: non-closed tasks, running agents, and not-merged PRs. UIs
SHALL render these counts rather than computing their own.

Each section SHALL also carry an **attention count**: how many of its rows wait on
the USER, a subset of the count beside it. Every such marker SHALL be derived here,
one recipe per section, so a new marker is a line in the section model rather than a
case in each view. A section holding nothing that can wait on a human SHALL carry no
recipe, and SHALL resolve to zero rather than to an error.

What crosses to a client SHALL be the **resolved** section: a key, a title and its
numbers. The recipe that derives a count from board state SHALL stay inside the hub,
because a function cannot cross the boundary; the hub SHALL resolve every count
against the board it is already serving, and SHALL carry the resolved sections on
that board, since a front-end may not link hub code and so cannot resolve them.

#### Scenario: A UI renders section counts

- **WHEN** a UI draws its tabs
- **THEN** each tab's badge is the hub-provided count for that section

#### Scenario: Adding a section

- **WHEN** a new section is introduced
- **THEN** it is added to the hub's section model and UIs pick it up without
  re-deriving counts

#### Scenario: Counts arrive resolved

- **WHEN** the board is served
- **THEN** each section carries its key, title, computed count and attention count,
  and no part of the derivation crosses to the client

#### Scenario: A section nothing can wait on

- **WHEN** a section has no attention recipe
- **THEN** its attention count is zero and its badge is drawn without a marker
