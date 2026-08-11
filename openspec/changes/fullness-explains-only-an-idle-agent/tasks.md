# Tasks

## 1. Make fullness explain rather than overwrite

- [x] 1.1 `overlayFullness` decides the word: `full` only when the status would otherwise be
      `idle` and the agent holds no task, no feature and no PR.
- [x] 1.2 Check held work directly rather than trusting the word — a quiet runtime probe reads a
      task-holder as `idle` before it has been still long enough to say `stalled`.
- [x] 1.3 Keep it a pure function beside `overlayRuntime`, so the rule is stated once and is
      testable without standing up a hub.

## 2. Say so where the field is documented

- [x] 2.1 The `AgentView` context fields in `internal/api/board.go` describe when the word appears.

## 3. Pin it

- [x] 3.1 Every status the overlay must leave alone, including `stalled` and the quiet
      task-holder. Mutation-checked: the test fails against the unconditional version.
- [x] 3.2 A not-full case for each status, so the overlay is only ever additive.
