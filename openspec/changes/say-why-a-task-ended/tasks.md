# Tasks

## 1. The ending carries its reason

- [ ] 1.1 Add `tasks.Ending` with the four values that exist — `EndingMerged`,
      `EndingCheckpointed`, `EndingClosedByUser`, `EndingScrapped` — and change
      `Source.Finish(root, id string, scrap bool)` to take one. A closed enum, not a string: an
      unrecognised reason must fail to compile rather than fall into a default.
- [ ] 1.2 `finishAtSource` takes and forwards the reason. Every caller states which it is —
      `CloseTask`, `ScrapTask`, `CmdCheckpoint`, and the merge path.
- [ ] 1.3 `td` and `github` sources: write today's behaviour per reason rather than leaving it to
      fall out of one branch. Where two reasons genuinely act alike, say so on the case.

## 2. An openspec change ends at its merge

- [ ] 2.1 `spec.Source.OnMerged` archives; `Finish` archives for nothing. Delete the comment that
      says a change is archived at close time — it is the decision being reversed, so it must not
      survive as a claim about the code.
- [ ] 2.2 `EndingScrapped` deletes the change, as today. `EndingCheckpointed` does nothing.
- [ ] 2.3 `EndingClosedByUser`: decide and STATE it. A hand-close with no merge is not "done" by
      this change's own rule, but leaving the change dir live re-lists the task as open on the next
      sync — so the answer is either "treat it as a scrap" or "archive it and say why the rule bends
      for a human". Do not leave this one implied.
- [ ] 2.4 A test that a checkpoint on an os- subtask leaves the change live and the task in the
      backlog, and that a merge archives it. This is the regression that cost a manual recovery.

## 3. Unfinished work is not ended quietly

- [ ] 3.1 `spec.Source` refuses to end a change whose own tasks.md is not fully ticked, naming the
      ratio. `Change.CompletedTasks`/`TotalTasks` already exist and already build the listing title
      — this reads the number the hub prints.
- [ ] 3.2 The refusal reaches the agent as an ANSWER, not a hub fault: an unfinished change is
      something the worker can act on, and returned bare it auto-escalates (-> AgentExec).

## 4. The verbs say what they do

- [ ] 4.1 Rename `checkpoint` to `done`. It ends the subtask and takes the next; the current name
      means a waypoint you pass through, which is the other verb.
- [ ] 4.2 `contribute` keeps its job; its help gains the contrast, so the two read as a pair —
      "lands work mid-task, the task stays yours" against "ends this subtask, takes the next".
- [ ] 4.3 Move the agent-facing text with it: the registry help, the durable brief, the directives
      naming the verb, and the feature-loop replies. A missed one tells an agent to run a verb that
      does not exist.
- [ ] 4.4 Delete the special case in `feature.go` that stops a checkpoint closing a task with open
      children ONLY if the completion refusal in section 3 covers it; if it does not, say what it
      still catches that the general rule misses.
