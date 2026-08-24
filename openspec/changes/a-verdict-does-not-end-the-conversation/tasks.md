# Tasks

## 1. Widen the scope, once

- [x] 1.1 `ProjectStore.RuledPRs` lists the PRs an author has recorded a verdict on, newest first —
      the counterpart to `ReviewingPR`, which is verdict-LESS by definition.
- [x] 1.2 `reviewerTasks` resolves the scope in one place: the held review's task, then the task of
      every PR ruled on, deduplicated, newest first.
- [x] 1.3 The gate, `commentTarget` and the explicit-id check all read it, so the verb cannot be
      offered by one rule and refused by another.

## 2. Say what is reachable

- [x] 2.1 The remaining refusal names what opens the verb — approving or rejecting a PR — rather than
      only that nothing is in reach.
- [x] 2.2 The id-refusal names the tasks that ARE reachable.
- [x] 2.3 The help offers both forms: a bare comment for the newest, an id for the rest.
- [x] 2.4 `MsgVerdictRecorded` names `comment` and the task, since it is read at the moment an
      afterthought arrives.

## 3. Keep the faults honest

- [x] 3.1 A store error still leaves the verb UNBLOCKED, so the error surfaces from the verb rather
      than reading as the settled "nothing to comment on".
- [x] 3.2 A missing row for the review currently HELD is still a hub fault and still says so; a missing
      row behind an OLD verdict is skipped rather than failing the verb.

## 4. Pin it

- [x] 4.1 A reviewer that has ruled can comment, and the verb is OFFERED — not merely functional,
      since a blocked verb is one an agent never tries.
- [x] 4.2 A held review outranks an older verdict for an id-less comment, and the older task is still
      reachable by name.
- [x] 4.3 A task it has never reviewed is still refused, and the refusal names what it can reach.
- [x] 4.4 A reviewer with nothing held and nothing ruled on is told what would open the verb.
