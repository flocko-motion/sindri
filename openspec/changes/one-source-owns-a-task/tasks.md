# Tasks

## 1. The port stops broadcasting

- [ ] 1.1 `Source` gains `Owns(taskID string) bool`, and the core resolves the owning source once
      (`ownerOf(project, id) (Source, error)`) instead of looping. An id no source owns is an error
      raised there, naming the id — one place, not a loop that fell through.
- [ ] 1.2 Delete the `handled`/`ok` returns from `Finish`, `Comments` and `AddComment`. A source
      that is asked owns the id, so its answer is the answer. `Comments` keeps a way to say "I hold
      no thread of my own" — that is a real distinction from "no comments yet" and must survive as
      its own signal rather than being folded into the deleted flag.
- [ ] 1.3 `OnMerged` is asked of the owner, not of everyone. Its doc says the opposite today, so
      delete that sentence rather than leave it as a claim about the code.

## 2. Finish carries the reason

- [ ] 2.1 Add a closed `tasks.Ending` — `EndingMerged`, `EndingSubtaskFinished`, `EndingClosedByUser`,
      `EndingScrapped` — replacing `scrap bool`. An unrecognised reason must fail to compile.
- [ ] 2.2 Every caller states which it is: `CloseTask`, `ScrapTask`, `CmdCheckpoint`, the merge path.
- [ ] 2.3 The three sources write today's behaviour per reason. Where two reasons genuinely act
      alike, say so on the case rather than letting it fall out of a shared branch.

## 3. A source can report unfinished work

- [ ] 3.1 `Source` gains a way to report that a task's work is incomplete, with the measure. The
      openspec source answers from `CompletedTasks`/`TotalTasks` — the number that already builds
      the listing title. td and github report nothing to say, today.
- [ ] 3.2 The close path asks the owner and refuses when it says unfinished, naming the ratio.
      Closing still ends a task at its source, unchanged, for every source: this adds a check
      BEFORE the close, and gives no source its own idea of when a task is done.
- [ ] 3.3 The refusal reaches the agent as an ordinary answer (exit non-zero, no error returned), or
      `AgentExec` turns it into a hub fault and stops the worker — which is exactly what happened.
- [ ] 3.4 A test for the incident: a checkpoint on a subtask whose change is at 2/10 is refused and
      names the ratio, and the change is still there afterwards.

## 4. The core stops branching

- [ ] 4.1 `ReopenTask` asks the owner whether it can reopen, instead of `task.IsOwned(id)`. Its
      refusal stops naming "an openspec change or a GitHub issue".
- [ ] 4.2 `TaskInfo` asks the owner where to read from, instead of `task.IsOwned(id)`.
- [ ] 4.3 The three strings in `CmdOpenspec` that name a tool — the "edit /workspace/openspec first"
      reply and the "openspec update" default subject — come from the source. The VERB keeps its
      name: it is a planner's landing verb and what the brief teaches, and the fluency is worth more
      than the purity. Say that on the command so it is not "cleaned up" later.

## 5. The verbs say what they do

- [ ] 5.1 Rename `checkpoint` to `done`. It ends the subtask and takes the next; the current name
      means a waypoint you pass through, which is the other verb.
- [ ] 5.2 `contribute` keeps its job; its help gains the contrast, so the two read as a pair.
- [ ] 5.3 Move the agent-facing text with it: registry help, the durable brief, the directives
      naming the verb, and the feature-loop replies. A missed one tells an agent to run a verb that
      does not exist.
- [ ] 5.4 Delete `feature.go`'s special case stopping a checkpoint closing a task with open children
      ONLY if section 3's refusal covers it. If it does not, say here what it still catches — two
      rules for one question is how the second instance was missed.

## 6. The line is held by a build

- [ ] 6.1 An arch test: no file under `internal/hub/workflow` or `internal/hub/*.go` names a task
      source in an identifier or a produced message, and none branches on an id scheme. Shaped like
      the two guards `internal/arch` already runs — enumerate, declare exceptions with a reason,
      fail on an undeclared one.
- [ ] 6.2 Agent-brief text about where a project keeps its specs is NOT an adapter reference; the
      test must not flag it. Say so in the test, since the next reader will ask.
