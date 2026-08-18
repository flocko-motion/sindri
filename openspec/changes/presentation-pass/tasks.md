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

- [x] 3.1 `prStatusStyle` derives RED from `api.PRNeedsUser` — the predicate the badge counts — so
      the colour and the badge cannot disagree.
- [x] 3.2 The rest of the mapping: grey finished, orange mid-merge, cyan the worker's rework, green
      a review that is coming. Yellow stays unused: no PR state is idle-but-unblocked.
- [x] 3.3 `stWorking` (cyan) is added as the shared "work is happening" colour, settled here because
      the PR mapping needs it before the task palette lands.
- [x] 3.4 The invariant is tested across every PR state: red exactly when counted.

## 4. Task row colours: red for stopped-waiting-on-you, positive for in-progress

- [x] 4.1 Unapproved AND unrated read red: both are gates only the user opens.
- [x] 4.2 In-progress reads cyan (`stWorking`), the colour PRs already use for rework in flight.
- [x] 4.3 Critical priority moves to pink, so red keeps one meaning across every tab.
- [x] 4.4 `taskRowStyle` is the one decision the rows make, and the test drives it rather than
      re-deriving it.
- [x] 4.5 The Tasks badge now counts both gates (`api.TaskNeedsUser`), so the invariant holds —
      the badge and the red row are one claim, and an unrated task was red under the old count
      without being counted.
- [x] 4.6 The palette header describes the scheme in force.
- [x] 4.7 The row ASKS `api.TaskNeedsUser` for red, as the PR row asks `api.PRNeedsUser`, instead of
      re-deriving it: the two parted on case order alone, and a rejected task with no rating was
      counted while rendering grey.
- [x] 4.8 A rejected task is out of both, rated or not: the user has ruled, and rating releases
      nothing while the rejection stands.
- [x] 4.9 The test board carries every combination of the two gates, not only the ones that agree.

## 5. Show the last-changed timestamp in the task detail

- [x] 5.1 `theme.When` composes the moment with its age once, since both front-ends had their own
      copy of that composition and the two timestamps must read alike side by side.
- [x] 5.2 "changed:" sits under "created:" in the TUI detail and in `sindri task info`.
- [x] 5.3 A source with no timestamp reads "n/a" — the blank IS the information, explaining why a
      mirrored task can be missing from the active filter, so nothing stands in for it.
