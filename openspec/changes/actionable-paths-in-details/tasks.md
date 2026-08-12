# Tasks

## 1. Make the agent's workspace actionable

- [x] 1.1 The Agents detail builds its workspace item from the absolute path, classified as a
      `path` — the single classification the existing focus, ENTER and yank handlers act on.
- [x] 1.2 Keep the field present, and plain, when there is no project root to join to.

## 2. Write the rule down

- [x] 2.1 `view-tui` gains the requirement, covering the PRs behaviour that was already
      implemented and unspecified alongside the new Agents one.
- [x] 2.2 Scope it to the agent workspace. A general "any path in a detail" rule would be false
      on landing: the Repos detail names a path and is built from plain strings, so it has no
      actionable item to reach.

## 3. Pin it

- [x] 3.1 A test that the workspace item is focusable, absolute, and yanked by `y`.
- [x] 3.2 A test that an unresolvable path keeps the field and drops the action.
