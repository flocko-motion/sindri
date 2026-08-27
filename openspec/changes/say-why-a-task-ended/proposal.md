# Say why a task ended, and let the verbs mean what they say

## Why

Four different events end a unit of work, and the code carries them as one boolean.

    finishAtSource(project, root, id string, scrap bool) error
    Source.Finish(root, taskID string, scrap bool) (handled bool, err error)

- a `checkpoint` finished a subtask inside a feature → `scrap=false`
- the user closed a task by hand → `scrap=false`
- the task's PR merged → a separate hook, `OnMerged`
- the user scrapped it → `scrap=true`

Three genuinely different events reach a source as the same `false`, so the source has to pick one
behaviour for all of them. `spec.Source.Finish` picked "archive the openspec change". That is right
for a merge and wrong for a checkpoint, and the wrong one is what fired: one `checkpoint` retired
`a-harness-command-answers` with **8 of its 10 tasks unticked**, the task vanished from the backlog
on the next sync, and the following checkpoint escalated its author over an id that no longer
resolved. The change had to be un-archived by hand.

The hub was not short of information. It computes the completion ratio for the listing — the title
is built as `"%s (%d/%d)"`, and printed `a-harness-command-answers (0/10)` — and never consults it
at the one moment it decides the work is over. The fact was collapsed at the seam; only the boolean
crossed.

**The verbs make the same mistake in the other direction: two of them are named the opposite of what
they do.**

- `contribute` — *"land an interim contribution mid-task"*. It is the waypoint: work goes to the
  reference, the task stays yours.
- `checkpoint` — *"record the current subtask and move to the next"*. It is TERMINAL: it commits,
  closes the subtask at its source, and advances the feature.

"Checkpoint" is the word that means a waypoint you pass through; "contribute" is the word that means
giving something to a shared thing. A worker looking for "save my progress and keep going" reads the
two names and picks the wrong one. That is exactly what happened, and the author was not mistaken
about anything except the vocabulary.

There is a scar for this already, in `feature.go`: *"A task with work under it can't be CLOSED by a
checkpoint (an epic was once closed over four open children)."* The same failure, previously, met
with a special case for open child tasks — which cannot see an openspec change's own unticked items.
Guarding the symptom is why it came back.

## What Changes

**The ending carries its reason.** `Finish(root, id, scrap bool)` becomes `Finish(root, id, Ending)`
where `Ending` names which of the four happened: `EndingMerged`, `EndingCheckpointed`,
`EndingClosedByUser`, `EndingScrapped`. Each source decides per reason instead of guessing:

- openspec: merged → archive; scrapped → delete; checkpointed → nothing, its PR ends it; closed by
  the user → its own decision, stated rather than inherited.
- td and github keep today's behaviour, now written per reason rather than falling out of one branch.

**"Done" for an openspec change means its PR merged.** The same rule the rest of the hub already
keeps for every task, and the hook to hang it on already exists — `OnMerged` was deliberately a
no-op, with a comment saying a change is archived at close time instead. That is the line to reverse.

**A source refuses to end work that is visibly unfinished**, and says the ratio it refused on. The
hub knows `CompletedTasks`/`TotalTasks`; a refusal that names them is a fact the caller can act on,
where an archive at 2/10 is silent damage.

**The verbs are renamed to what they do.** `checkpoint` becomes `done` (it ends the subtask and
takes the next); `contribute` keeps its job and gains the description that says it is the mid-task
one. A worker choosing between them then chooses on meaning rather than on a guess.

## Impact

- Specs: `05-workflow` gains what ends a unit of work and what each verb means.
- Code: `internal/adapter/tasks/tasks.go` (the `Ending` type and the interface),
  `internal/adapter/tasks/spec`, `.../td`, `.../github`, `internal/hub/workflow/close.go`,
  `feature.go`, `pr.go`, `merge.go`, `internal/hub/commands.go` and the agent brief.
- The rename is agent-facing: the durable brief, the registry help and the directives all name these
  verbs, so they move together or an agent is told to run something that does not exist.

## Where this sits

Independent of `sd-badd07` (the harness seam) and can be worked beside it, with one caution: both
touch `internal/hub/workflow/task.go`. It is the same class of defect — a fact collapsed into a
boolean at a seam — in the work's lifecycle rather than the agent's session.
