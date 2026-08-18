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
- (Sibling subtasks of this feature add the reviewer's author-in-directive and the task views'
  holder column; each checkpoints its own account of what changed.)

## Impact

- Specs: `05-workflow` (a planner's view of its own staff).
- Code: `internal/hub/commands.go` (the `staff` verb and its registry entry).
- Not an address book: nothing here enumerates agents outside the caller's own project, and no verb
  that would is added.
