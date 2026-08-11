# project-config — delta

## MODIFIED Requirements

### Requirement: GitHub issue source toggle

The GitHub issue source (see the `github-issues` capability) SHALL be configurable per
project via the `github.issues` boolean key, defaulting to **on**. This key is the
per-project surface that capability defers to: when unset or `true` the source is active for
the project (still subject to `gh` availability and graceful absence); when `false` no issues
are imported. The opt-out direction is deliberate — a repository shows its issues without the
user first discovering a flag, and the unrated-import rule keeps that from becoming surprise
work.

#### Scenario: Source on by default

- **WHEN** `github.issues` is unset and `gh` is available with a GitHub remote
- **THEN** the project's open GitHub issues are imported

#### Scenario: Source disabled explicitly

- **WHEN** `github.issues: false`
- **THEN** no issues are imported, regardless of `gh` availability

#### Scenario: Enabled explicitly

- **GIVEN** `github.issues: true` in `.sindri/config.yaml`
- **WHEN** the hub syncs tasks and `gh` is available with a GitHub remote
- **THEN** the project's open GitHub issues are imported
