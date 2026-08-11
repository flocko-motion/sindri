# Tasks

## 1. Render the thread for an agent

- [x] 1.1 One renderer in `internal/hub/workflow`, shaped like the CLI's so the two read alike.
- [x] 1.2 `task <id>` renders the comments `TaskInfo` already attached.
- [x] 1.3 Bare `task` fetches the thread — it reads the task row, and comments live in their own
      table — and renders it for a standalone task and for a package's own thread.

## 2. Name the source everywhere

- [x] 2.1 The TUI's thread gains the source on its head line; the CLI already printed it.

## 3. Pin it

- [x] 3.1 Tests for both agent-facing paths, for the planner reading the same view, and for the
      empty thread rendering nothing. Mutation-checked compilably — the four rendering tests fail
      against the old code.
- [x] 3.2 A TUI test for the source, and one for the empty thread.
