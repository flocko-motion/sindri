# view-tui — delta

## MODIFIED Requirements

### Requirement: Agents and PRs tabs have a global/repo scope toggle

The Agents and PRs tabs SHALL each offer a scope toggle between `global` and a narrow scope,
defaulting to `global`. In `global` the tab SHALL show the whole fleet across every registered
repo, each row repo-tagged. The active scope SHALL be shown in the footer. This is a view filter
only; it SHALL NOT change what data the hub holds, and the Tasks tab SHALL remain always scoped to
the active repo.

The narrow scope SHALL show the active repo's entries PLUS any entry from another repo that is
waiting on the user. Agents and PRs are BACKGROUND work: they progress while the user is looking
somewhere else, which is exactly why something that ends up waiting on them has to surface wherever
they are. An approved pull request in another repo is the one action only the user can take, and a
scope that hid it left them told that something needed them and shown a list where nothing did.

Rows from another repo SHALL be grouped by repo rather than interleaved, so a foreign row reads as
what it is rather than as a filter that has stopped working.

Whether an entry is waiting on the user SHALL be decided by ONE predicate per kind, the same one
the attention marker and the row colour read. Visibility, colour and count derived separately drift,
and the drift is invisible because each looks plausible alone — a row shown here, counted in the
badge, and coloured as though nothing were owed.

The narrow scope's label SHALL say what it does. It admits entries from outside the active repo, so
a label naming the repo alone would be a filter claiming to exclude what it plainly shows.

This SHALL NOT extend to the Tasks tab. A proposal awaiting a verdict is FOREGROUND work: it was
created in the repo the user is already in, and they will see it because they are there.

#### Scenario: Default is global

- **WHEN** the Agents or PRs tab is first shown
- **THEN** it lists entries across all repos, each tagged with its repo

#### Scenario: Narrow to the active repo

- **WHEN** the user toggles the Agents tab scope to the narrow scope
- **THEN** it shows the active repo's agents, and the footer reflects the narrow scope

#### Scenario: A pull request waiting on the user crosses repos

- **WHEN** the PRs tab is narrowed to the active repo and another repo holds a PR waiting on the
  user
- **THEN** that PR is listed, grouped under its own repo, and counted in the tab's badge

#### Scenario: The narrow scope still excludes

- **WHEN** another repo holds a PR that nothing is asked of the user for
- **THEN** it is not listed in the narrow scope

#### Scenario: The Tasks tab is unaffected

- **WHEN** another repo holds a task awaiting the user's verdict
- **THEN** the Tasks tab still shows only the active repo's tasks
