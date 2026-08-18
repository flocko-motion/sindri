# Tasks

## 1. Planners see the staff of their own repo (sd-3467ab)

- [x] 1.1 A `staff` verb, planner-only, lists the project's roster with role and what each holds.
- [x] 1.2 Scoped by construction: the roster query is the caller's own project, so there is no
      fleet-wide path to close off later.
- [x] 1.3 What is held is read the way each role actually holds it — a reviewer's open review takes
      precedence, then a feature/task, then nothing — and retirement is said, since it decides
      whether to wait.
- [x] 1.4 Pinned: names and roles present, another project's agents absent, retired vs. idle read
      differently, and the verb is offered to a planner and nobody else.

## 2. The review directive names the worker (sd-37fd79)

- [x] 2.1 The directive's opening line names the author beside the task, at both call sites — the
      fresh claim and the re-ask, the second being the easy one to leave behind.
- [x] 2.2 An authorless record falls back to the previous wording rather than claiming an author.
- [x] 2.3 Pinned on the wording and on the wiring: the directive the reviewer is actually handed
      carries the PR's own author.
- [x] 2.4 The "ask the author instead of rejecting" line is deliberately NOT added: there is no
      send verb yet, and a comment does not reach the holder. It belongs with sd-2f38d9.

## 3. Task views name the agent holding the task (sd-4e4d5a)

- [ ] (next subtask)
