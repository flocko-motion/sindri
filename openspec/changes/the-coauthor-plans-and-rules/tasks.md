# Tasks

## 1. Grant the verbs

- [x] 1.1 `create-task` and `edit-task` admit a coauthor — the backlog it already reads.
- [x] 1.2 `approve` and `reject` admit a coauthor, and `approve`'s per-role help says what its
      verdict IS: a real one, recorded under its name, with nothing ever handing it a review.

## 2. Make the verdict a verdict, and attribute it

- [x] 2.1 A coauthor's approval writes its own badge (`AddVerdict`), since `completeReview` stamps only
      an assigned review row and a coauthor will never hold one — an approval with no author is what
      the badge model exists to stop.
- [x] 2.2 A coauthor's rejection writes its badge the same way, and `reject` takes the VOICE that ruled
      rather than a user/reviewer flag: the author is told `[<coauthor>]`, and the delivery's sender
      says so structurally rather than through the wording.
- [x] 2.3 A coauthor's verdict leaves its resting state alone — no `idle`, no nudge to ask for the next
      review, since neither is true of an agent standing with the user.

## 3. Hold the one rule that survives

- [x] 3.1 No agent rules on a PR built from its own commits, on either verb, for every role.
- [x] 3.2 The refusal names the allowed half — a task it WROTE is different — so the softened guard
      does not read as an oversight.

## 4. Tell the role what it holds

- [x] 4.1 The coauthor's durable brief and its directive name the backlog verbs and the PR verdicts. A
      grant an agent is never told about is no grant: its brief listed three optional helpers and
      nothing else.

## 5. Pin it

- [x] 5.1 A coauthor's approval: status approved, one non-advisory badge with its name and a time, its
      phase still `collab`, and nothing injected at it.
- [x] 5.2 A coauthor's rejection: the PR rejected with the feedback, a `changes` badge under its name,
      and the author told in that name with the sender recorded.
- [x] 5.3 Own commits refused on both verbs for a coauthor, a reviewer and a planner, with nothing
      written and no badge recorded.
- [x] 5.4 The queue hands a coauthor nothing: `AssignPendingReviews` leaves the row unclaimed and the
      directive is still the coauthor's own.
- [x] 5.5 A coauthor's `create-task`/`edit-task` behave as a planner's — pending approval on both.
- [x] 5.6 The registry surface: the four verbs granted, `next`/`submit`/`checkpoint` still refused, and
      a worker gaining none of them.
