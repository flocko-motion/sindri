## MODIFIED Requirements

### Requirement: The hub hosts the logic; UIs and agents are clients

Application logic SHALL be hosted by a single global hub process — the one writer of
external state for every repository it serves. All user interfaces (CLI, TUI) and all
agents SHALL be thin clients of that hub, passing the repo (project) each request
concerns, rather than mutating external state themselves. The hub SHALL keep its state
centrally, outside any repo, never per-repo on disk.

#### Scenario: A UI changes state

- **WHEN** a CLI or TUI action mutates state for a repo
- **THEN** it calls the global hub with that repo's context, which performs the
  change, rather than writing td/git/the store itself
