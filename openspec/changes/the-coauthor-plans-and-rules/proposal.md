# The coauthor gains task authorship and PR verdicts

## Why

The coauthor is the strongest role by design — the deliberate exception to agent isolation, driven
directly by the user, holding no queued work — and it could read everything and decide nothing. It
already sees the whole backlog like a planner and could change no part of it. It could list a PR, read
its diff and run the quality gate against it, and had nowhere to put what it concluded.

Both halves of that are reach the role is meant to have. Giving them to a coauthor gives them to a
human in the loop rather than to an autonomous actor: it is steered turn by turn by the user, in the
user's own checkout, and neither verb reaches a gate the user does not still hold.

## What changes

- **Task authorship.** `create-task` and `edit-task` are open to a coauthor. Every proposal still
  waits for the user's approval before any worker can claim it, and every edit still returns the task
  for a fresh one — the grant is reach into the backlog, not permission over it.
- **PR verdicts.** `approve` and `reject` are open to a coauthor, and its approval is a real one: not
  the planner's advisory badge, which exists because a planner rules on its own plan. Merge is
  human-only, so a coauthor's approval lands nothing by itself.
- **The verbs, never the queue.** Nothing hands a coauthor a review. Both assignment paths already
  require the reviewer role (`reviewerAssignable`, `idleReviewer`), and a test now pins it, because a
  coauthor that could be handed a review would be waiting on the fleet instead of on the user.
- **Its verdict is attributed.** A reviewer's badge lands on the review row it was assigned; a
  coauthor holds no such row, so its badge is written outright under its own name. Its rejection
  reaches the author in its own voice — `[brokk]`, not `[reviewer]` — since an author weights feedback
  by who it is from, and a coauthor speaks for nobody but itself.
- **Its resting state is left alone.** A reviewer's verdict returns it to `idle` and nudges it to ask
  for the next review. Both would be false told to a coauthor, which is standing with the user.

## Attribution rather than prohibition

The earlier plan blocked a coauthor from approving a PR built from a task it wrote. That block is
dropped, decided with the user: merge is human-only, the badge model records who approved and when, so
"approved by the coauthor that also wrote this task" is legible and the user weighs it. Visibility
informs the decision; prohibition removes an option and makes the role a weaker reviewer without
making the system safer.

One rule does still hold, and now holds for every role: no agent rules on a PR built from its OWN
COMMITS. The refusal says which half of the rule it is, so the allowed half is not read as an
oversight.

## `05-workflow`'s wording, sharpened

"No agent SHALL approve or merge its OWN work" has now been read two ways twice (here and in
sd-121327): work it COMMITTED, or work it planned. It means the commits. The requirement now says so,
and says that authoring the plan is not authoring the work — with the badge as the answer to the
provenance that leaves behind.

## What is deliberately not here

- **No `prioritise-task` or `reopen-task`.** Authorship is proposing and revising. Pacing the backlog
  and overturning the user's own verdict are theirs.
- **No review queue membership, and no watchdog.** A stalled coauthor is a user sitting at a prompt.
- **No notification when the user rules on a coauthor's task.** `RejectTask` tells planners, since
  they are the role that proposes and then goes idle. A coauthor is in the room with the user, who
  says it directly — a fact worth knowing rather than plumbing.

## Impact

- Specs: `05-workflow`'s coauthor requirement gains what it may now do, and the plan/build/review
  requirement's self-review rule is sharpened to name the commits.
- Code: `internal/hub/commands.go` (the grants and `approve`'s help), `internal/hub/workflow/verdict.go`
  (the own-commits guard, the coauthor's badge, the rejection's voice),
  `internal/hub/workflow/injected.go`, `internal/hub/workflow/prompts.go` (the brief and the directive
  now name the verbs the role holds — a grant an agent is never told about is no grant).
