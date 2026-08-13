# Tasks

## 1. PR detail: a lifecycle summary at the top

- [x] 1.1 Classify every logged PR event as milestone or diagnostic, in `internal/api`.
- [x] 1.2 `api.PRLifecycle` projects the log into milestones, verdict authorship from the review
      records rather than the payload prose.
- [x] 1.3 Both front-ends render it above the diff; the full log stays below.
- [x] 1.4 A test walks every `LogPR` call in the hub and fails on an unclassified type — and the
      other direction, so a classified type nothing logs is caught too.

## 2. Label an unapproved task "unapproved", not "pending"

- [x] 2.1 `theme.ApprovalLabel` supplies the word, beside PriorityLabel and StateLabel — approval
      was the one gate missed when presentation moved out of the API.
- [x] 2.2 Applied in the CLI's `task list` and `task info`, and in the TUI's rows and detail.
- [x] 2.3 The stored value is untouched: every predicate still branches on "pending".
- [x] 2.4 The row tests assert the shared label rather than a literal, so the word cannot drift.

## 3. Colour PR rows on the same rule as tasks and agents

- [ ] 3.1

## 4. Task row colours: red for stopped-waiting-on-you, positive for in-progress

- [ ] 4.1

## 5. Show the last-changed timestamp in the task detail

- [ ] 5.1
