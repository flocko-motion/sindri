# Colleagues are named: who worked on what

## Why

Agents work anonymously to each other, and it blocks the obvious follow-up: a planner or reviewer
who wants to reach the person who did the work has no name to reach.

What this opens up is context knowledge of direct colleagues, not fleet-wide access. A reviewer
learns the author of the PR in front of it. A planner learns who holds the tasks it planned, and who
else works in its own repo — its repo, not the fleet.

Visibility stays local; addressing is global. Mail reaches any agent by bare name, but a name from
another repo arrives through the user's instruction — "explain this to the planner of the library
repo, dwalin" — rather than through a lookup. An agent can talk to someone it has been told about,
and cannot go browsing the fleet.

## What changes

- A planner gains `staff`: its own repo's roster, each agent's role, and what it currently holds —
  a review, a task, a feature, or nothing, with retirement said plainly since it decides whether to
  wait for someone. Scoped to the project by construction: the query is the planner's own roster,
  never the fleet's.
- The review directive names the author: "Review pr-sd-1 — dwalin's work on task sd-1234", in the
  sentence that says what to do rather than in a footnote. A record with no author reads as it did
  before, since "'s work on" with nothing in front of it would be worse than the line it replaced.
- (The remaining sibling subtask adds the holder to the task views.)

## Impact

- Specs: `05-workflow` (a planner's view of its own staff; the author in the review directive).
- Code: `internal/hub/commands.go` (the `staff` verb and its registry entry),
  `internal/hub/workflow/prompts.go` and `review.go` (the author in the review directive).
- The follow-up the subtask anticipates — a reviewer asking the author instead of rejecting on a
  guess — is NOT promised in the directive yet: agents can read mail but cannot send it, and a
  comment on the task only wakes the board, not its holder. The sentence belongs with the verb that
  makes it true (sd-2f38d9).
- Not an address book: nothing here enumerates agents outside the caller's own project, and no verb
  that would is added.
