# A verdict does not end the reviewer's conversation

## Why

A reviewer loses `comment` at the exact moment it acquires afterthoughts.

The gate asks `ReviewingPR`, which is documented as the newest VERDICT-LESS review assigned to the
agent. Recording the verdict is what ends the review, so the verb shuts on the same call that submits
the rejection — and `commentTarget` resolved the same way, so there was nothing to write to either.
The case that exposes it is ordinary: the user opens a chat with a reviewer after its rejection, or the
reviewer simply thinks of something more — a second look at a design choice, a consequence it missed.

Its only remaining channel was `mail <worker> <message>`, and mail is the wrong home for a finding. A
comment lives on the task, and every agent that reads the task gets it: a later worker, a second
reviewer, a planner, or the same reviewer after a compaction. Mail is addressed to one agent, nobody
else sees it, and the recipient's memory of it dies at the next compaction — the row persists, but
nothing leads a future reader to it. So the reviewer was being pushed to the one channel that produces
no durable record.

## What changes

- **A reviewer may comment on any task it has recorded a verdict on**, as well as the review it holds.
  The gate exists so a reviewer cannot wander the backlog commenting on work it knows nothing about,
  and that stays true: having ruled on a PR is proof it read that work. Only the cliff at the verdict
  goes.
- **One reader for the scope.** The gate that offers the verb, the resolution of an id-less comment,
  and the check on an explicit id now all read `reviewerTasks` — newest first, the held review ahead of
  the latest verdict. Three readers of a scope, disagreeing, is a verb offered and then refusing
  everything.
- **An id is worth typing now**, so the help offers both forms: with more than one verdict behind it, a
  bare comment means the newest and the others are reachable only by name.
- **The refusal names what would reach one.** "You aren't reviewing a PR, so there's no task to comment
  on" described the wall; a reviewer holding nothing still has everything it has ruled on, and the one
  remaining refusal says what opens the verb.
- **The verdict receipt says the door is open.** `MsgVerdictRecorded` is what a reviewer reads at
  exactly the moment an afterthought arrives, so it now names `comment` and says the task is where a
  later reader finds it. A widened gate nobody is told about is a gate nobody walks through.

## Scope kept as it was

- A PR whose row has gone leaves its old verdict out of the scope rather than failing the whole verb; a
  MISSING row for the review the agent currently HOLDS is still a hub fault and still says so.
- Nothing else moves. `mail` remains right for what it is for — telling an agent something it must read
  that has no task or PR to sit on.

## sd-4e3d91 is the same principle, and is not in this change

That task asks for rejection mail to become a pointer with the body staying on the PR, for the reason
this one settles: one canonical place a later reader can find, and a short message whose job is only to
wake somebody. It is a separate task and not part of this feature, so it is left to whoever takes it —
the principle is now written down here for them to point at.

## A note on where the base requirement lives

The scope sentence being widened ("a reviewer MAY comment only on the task of the PR it is reviewing")
is not in the current specs at all: it lives in the unarchived `agents-can-comment-on-tasks` change. So
this change ADDS its requirement rather than modifying that sentence. If that change is archived AFTER
this one, its reviewer clause has to be updated as it lands, or the older wording will arrive as the
newer word.

## Impact

- Specs: `05-workflow` gains a requirement for how long a reviewer's reach lasts.
- Code: `internal/hub/commentscope.go` (new), `internal/hub/commands.go`,
  `internal/hub/store/workflow.go` (`RuledPRs`), `internal/hub/workflow/injected.go`.
