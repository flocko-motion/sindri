# project-config Specification

## Purpose
TBD - created by archiving change add-project-config. Update Purpose after archive.
## Requirements
### Requirement: Per-project config file and precedence

A project SHALL be configurable through a `.sindri/config.yaml` file at its
repository root, read by the hub when it resolves the project. A repo-level
config SHALL take precedence over an optional global `config.yaml` under the hub's
state directory, which in turn SHALL take precedence over the built-in defaults —
resolved per top-level key (a key set in the repo config overrides the same key in
the global config). This mirrors the existing custom-Containerfile precedence.

#### Scenario: Repo config read

- **WHEN** the hub resolves a project whose repo has `.sindri/config.yaml`
- **THEN** the settings in that file apply to that project

#### Scenario: Repo overrides global

- **WHEN** a key is set in both the repo config and the global config
- **THEN** the repo config's value wins for that key

#### Scenario: No config file

- **WHEN** a project has no `.sindri/config.yaml` (and no global config)
- **THEN** every setting takes its documented default and no error is raised

### Requirement: Invalid config fails loud, never silently accepted

Invalid config SHALL cause the operation that needs the project — launching an
agent, syncing tasks, building the image — to fail with a clear, actionable error
that names the config file and the specific problem. The hub SHALL NOT ignore a
bad config or fall back to defaults. Because the hub serves multiple projects, the
failure SHALL be scoped to operations on the affected project and SHALL NOT take
down the hub or other projects. The following SHALL each be invalid config:
malformed YAML; an unrecognized key; a value of the wrong type; and a path value
that is absolute, escapes the repository root, or (when the key is set) points at
a file that does not exist.

#### Scenario: Malformed YAML

- **WHEN** `.sindri/config.yaml` is not valid YAML
- **THEN** the affected operation fails with an error naming the file, and no
  default is silently substituted

#### Scenario: Unknown key

- **WHEN** the config contains a key sindri does not recognize
- **THEN** it is rejected as invalid config with an error naming the key

#### Scenario: Configured path is missing

- **WHEN** a path key (e.g. `architecture`) names a file that does not exist
- **THEN** the affected operation fails with an error naming the key and path

#### Scenario: Path escapes the repo

- **WHEN** a path value is absolute or contains `..` that escapes the repo root
- **THEN** it is rejected as invalid config

#### Scenario: Absent config is valid

- **WHEN** there is no config file at all
- **THEN** this is not an error — defaults apply (see the no-config scenario above)

### Requirement: Configurable architecture-doc path

The path to the architecture document the reviewer is told to read SHALL be
configurable via the `architecture` key, as a repo-relative path (e.g.
`openspec/architecture.md`). When the key is unset the default SHALL be
`ARCHITECTURE.md` at the repo root. When the key is set the reviewer prompt SHALL
point at the configured path and the file MUST exist (a missing configured doc is
invalid config).

The hub SHALL NOT create an architecture document in any repo, at either the
default or a configured path — the doc is the project's to write. A repo with no
readable doc SHALL be reported as a recommendation (never an error), and agents
SHALL work without an architecture brief in that case.

#### Scenario: Custom architecture path

- **GIVEN** `architecture: openspec/architecture.md` and that file exists
- **WHEN** a reviewer is assigned a PR
- **THEN** the reviewer prompt tells it to read `/workspace/openspec/architecture.md`

#### Scenario: Missing default doc is recommended, never written

- **WHEN** no `architecture` key is set and the repo has no `ARCHITECTURE.md`
- **THEN** the hub writes no file into the repo
- **AND** it recommends, at hub startup and in the Repos tab detail, that the user
  point `architecture` at the repo's own doc
- **AND** agents for that repo are briefed without an architecture section

#### Scenario: Configured path missing is invalid

- **GIVEN** `architecture: docs/arch.md` and that file does not exist
- **WHEN** the project is resolved
- **THEN** it is rejected as invalid config (the hub does not create `docs/arch.md`)

### Requirement: Configurable image recipe path

The agent image recipe SHALL be settable via the `containerfile` key, a
repo-relative path that takes precedence over the magic-filename discovery. When
the key is set the file MUST exist and that recipe SHALL be used to build the
image; when unset the existing discovery (`.sindri/{Containerfile,Dockerfile}`,
then the global recipe, then the embedded default) SHALL remain the fallback.

#### Scenario: Explicit containerfile path

- **GIVEN** `containerfile: .sindri/agent.Dockerfile` and that file exists
- **WHEN** the hub builds the project's agent image
- **THEN** it uses that file as the recipe

#### Scenario: Unset falls back to discovery

- **WHEN** no `containerfile` key is set
- **THEN** the magic-filename discovery and embedded-default fallback behave as
  they do today

### Requirement: Configurable reviewer prompt path

The reviewer prompt SHALL be overridable via the `review_prompt` key, a
repo-relative path whose file contents replace the default reviewer prompt. When
the key is set the file MUST exist; when unset the existing default reviewer
prompt SHALL be used.

#### Scenario: Custom reviewer prompt

- **GIVEN** `review_prompt: .sindri/review.md` and that file exists
- **WHEN** a reviewer is assigned a PR
- **THEN** the reviewer prompt is built from that file's contents

#### Scenario: Unset uses the default prompt

- **WHEN** no `review_prompt` key is set
- **THEN** the default reviewer prompt is used

### Requirement: GitHub issue source toggle

The GitHub issue source (see the `github-issues` capability) SHALL be enabled per
project via the `github.issues` boolean key, defaulting to `false`. This key is
the per-project opt-in surface that capability defers to: when `true` the source
is active for the project (still subject to `gh` availability and graceful
absence); when `false` or unset no issues are imported.

#### Scenario: Source enabled by config

- **GIVEN** `github.issues: true` in `.sindri/config.yaml`
- **WHEN** the hub syncs tasks and `gh` is available with a GitHub remote
- **THEN** the project's open GitHub issues are imported as todos

#### Scenario: Source off by default

- **WHEN** `github.issues` is unset or `false`
- **THEN** no GitHub issues are imported, regardless of `gh` availability

