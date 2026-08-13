# Tasks

## 1. Grow the unit instead of stranding it

- [x] 1.1 `adoptChild` runs wherever a parent link is written — task creation and re-parenting
      alike, since both add a child.
- [x] 1.2 A leaf holder is promoted to hold the task as a feature, same branch, via
      `promoteToFeature`.
- [x] 1.3 The promotion assigns nothing: it parks the agent and the ordinary directive picks the
      subtask. Assigning there read the approval gate before the row existed and handed out
      unreleased work — caught by the pending-child test.
- [x] 1.4 The agent is told, with a different note for each case: promoted, or the child landed
      inside the feature it already holds.

## 2. Remove the dead end

- [x] 2.1 `CmdCheckpoint` records and advances past a task with open children instead of refusing;
      the task stays open and `closeCompletedAncestors` finishes it when its children are.
- [x] 2.2 `CmdSubmit` on a grown leaf extends rather than refuses — promote, say what is held, point
      at the subtask.

## 3. Hold the invariant at the merge

- [x] 3.1 A merge whose task has open children lands as a milestone; the task stays open and the
      worker keeps it. Promoted there too, so the milestone runs on the one path that resumes an
      agent inside a feature.
- [x] 3.2 `store.OpenChildIDs` counts every unfinished status, not the literal `open`. A child being
      WORKED was invisible to close, reconcile, checkpoint and both new guards.

## 4. Decide the two edges

- [x] 4.1 NESTING: one level down needs nothing. The feature loop already serves leaves at any depth
      and closes intermediate parents, so the agent keeps its state and only the strand is removed.
- [x] 4.2 A CHILD THAT SHOULD NOT BLOCK: every open child blocks; the release is to REJECT it, which
      is on record, reversible, and already blocks neither pool. No silent non-blocking child.

## 5. Pin it

- [x] 5.1 A gained child promotes, tells, and is handed over by the directive.
- [x] 5.2 A gained child awaiting a verdict is not handed out and holds the feature open.
- [x] 5.3 A subtask that gains a child keeps its holder's state, tells it, and the checkpoint
      carries on rather than refusing.
- [x] 5.4 A submit of a grown leaf opens no PR and leaves the agent holding the feature.
- [x] 5.5 A merge never closes a task over an open child. Mutation-checked: without the guard the
      task closes and the worker is released, which is the original incident.
