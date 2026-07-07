# view-tui — delta

## MODIFIED Requirements

### Requirement: Repo switcher scopes the per-repo view

The TUI SHALL make the active repo a first-class, always-visible part of the
interface, and SHALL provide a switcher to change it.

The **active repo name SHALL be persistently visible** in the top bar (not only
inside an overlay), rendered in that repo's deterministic color scheme, so the user
can tell at a glance which repo the Tasks tab and any `repo`-scoped view reflect.

The **switcher SHALL be a picker overlay, not a tab strip** — the number of repos
may be large, so a fixed tab row would not scale. The overlay SHALL list the
registered projects (from the project registry) ordered most-relevant first: repos
with **live agents** on top, then by **recency** (last used), then the rest; and it
SHALL offer a typeahead filter to narrow a long list. Selecting a repo SHALL scope
the Tasks tab (and any other per-repo view) to it.

#### Scenario: Active repo always visible

- **WHEN** the TUI is showing any tab
- **THEN** the current repo's name is visible in the top bar, in that repo's color

#### Scenario: Switching repos

- **WHEN** the user opens the switcher overlay and picks a repo
- **THEN** the per-repo view rescopes to it and the top-bar indicator updates

#### Scenario: Switcher ordering and scale

- **WHEN** the switcher overlay is opened with many repos registered
- **THEN** repos with live agents appear first, then by recency, and a typeahead
  filter is available — it is a scrollable list, never a fixed tab row

#### Scenario: Rows carry their repo

- **WHEN** the Agents or PRs tab lists entries from more than one repo
- **THEN** each row shows which repo it belongs to

## ADDED Requirements

### Requirement: Agents and PRs tabs have a global/repo scope toggle

The Agents and PRs tabs SHALL each offer a scope toggle between `global` and `repo`,
defaulting to `global`. In `global` the tab SHALL show the whole fleet across every
registered repo, each row repo-tagged. In `repo` the tab SHALL show only the active
repo's entries (the switcher's selection). The active scope SHALL be shown in the
footer. This is a view filter only; it SHALL NOT change what data the hub holds, and
the Tasks tab SHALL remain always scoped to the active repo.

#### Scenario: Default is global

- **WHEN** the Agents or PRs tab is first shown
- **THEN** it lists entries across all repos, each tagged with its repo

#### Scenario: Narrow to the active repo

- **WHEN** the user toggles the Agents tab scope to `repo`
- **THEN** it shows only the active repo's agents, and the footer reflects `repo`
  scope

### Requirement: Repo configuration is editable in the TUI

The TUI SHALL let a user edit a repo's `.sindri/config.yaml` through a form over its
keys (`architecture`, `containerfile`, `review_prompt`, `github.issues`), performed
through the hub. An invalid entry SHALL be reported to the user and SHALL NOT be
persisted as a broken config; hand-editing the YAML file directly SHALL remain
equally valid.

#### Scenario: Edit config via a form

- **WHEN** the user opens the repo config form, changes a value, and saves
- **THEN** the hub writes `.sindri/config.yaml` and the change takes effect on the
  next load

#### Scenario: Invalid config rejected

- **WHEN** the user enters a value that fails config validation (e.g. a path that
  escapes the repo)
- **THEN** the form reports the error and does not persist a broken config
