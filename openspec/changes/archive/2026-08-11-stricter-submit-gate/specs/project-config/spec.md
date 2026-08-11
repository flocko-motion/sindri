# project-config — delta

## ADDED Requirements

### Requirement: Configurable submit-gate command

A project SHALL be able to declare its own quality gate through a `verify` key: a
repo-relative path to an executable that the submit gate runs in the agent's worktree,
in addition to the built-in checks (see `03-gh-local`). The key SHALL be validated like
every other path key — absolute, escaping the repository root, or naming a file that
does not exist is invalid config, and fails loudly rather than falling back.

When the key is unset the built-in gates apply alone, so an existing project's
behaviour is unchanged. The value SHALL be a path rather than a command line, so it can
be validated before it is run and so the project owns its invocation: a project whose
gate is a build tool wraps it in a script.

#### Scenario: Declared gate runs at submit

- **GIVEN** `verify: scripts/verify.sh` and that file exists
- **WHEN** an agent submits work
- **THEN** the script runs in the agent's worktree and its exit status decides whether
  the PR is created

#### Scenario: Unset key keeps today's behaviour

- **WHEN** no `verify` key is set
- **THEN** the submit gate runs the built-in checks only, as it does today

#### Scenario: Missing gate file is invalid config

- **GIVEN** `verify: scripts/verify.sh` and that file does not exist
- **WHEN** the project is resolved
- **THEN** it is rejected as invalid config naming the key and the path, and no submit
  silently proceeds ungated

#### Scenario: Path escaping the repo is rejected

- **WHEN** the `verify` value is absolute or escapes the repository root
- **THEN** it is rejected as invalid config, like every other path key
