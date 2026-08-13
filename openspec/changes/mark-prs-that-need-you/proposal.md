# Mark the PRs that cannot move without you

## Why

The Tasks and Agents handles carry a `(N!)` marker for what waits on the user; the PRs handle does
not, and PRs are where the waiting is most expensive. An approved PR is finished work: every gate it
had to pass, it has passed, and it sits unmerged because merging is human-only and nobody looked. An
open PR with no reviewer running is worse — it is not queued behind anything, it is simply never
going to be reviewed, and its row looks exactly like one being worked on.

## What changes

- `api.PRNeedsUser` states the rule: approved (waiting on the merge), or open with no reviewer alive
  anywhere (waiting on a review that is not coming). A rejected PR waits on its author, so it is
  out.
- A third state joins those two: an open **interim** PR — a mid-task contribution or a milestone.
  No reviewer is ever asked for one (`workflow.needsReview` excludes them, `resolve.go` says
  "interim PRs are user-gated — no reviewer"), so it is the user's from the moment it opens, and a
  milestone blocks its agent until the merge lands. The task named the first two; this is the same
  rule applied to a case the code already documents, and it is the one where an agent is stopped
  dead meanwhile. Counting it is a judgement call worth disagreeing with — say so and it comes out.
- The reviewer test is fleet-wide, not per PR. An unassigned PR is ordinary while a reviewer runs —
  it picks one up shortly — so assignment says nothing on its own; the absence of every live
  reviewer is what strands the queue. `api.AnyLiveReviewer` names that, over `AgentNotUp`'s existing
  list of words meaning "no pod".
- The PRs section gains its attention recipe in `hub/commands`, beside the two already there. The
  TUI needed no change at all: it has drawn every handle in one loop over the resolved sections
  since the groundwork landed, which is what that shape was for.
- `sindri pr list` marks each waiting row with why it waits, and closes with a line naming the PRs
  grouped by what clears them — the two states need different commands, so one verdict word per row
  would send the reader to the wrong verb half the time.

## Impact

- Specs: `05-workflow` gains a requirement for what waits on the human, beside the merge gate it
  follows from. The section-model requirement in `hub` already covers the mechanism.
- Code: `internal/api/attention.go`, `internal/api/board.go`, `internal/hub/commands/sections.go`,
  `internal/ui/cli/hub.go`.
- `sindri pr list` now reads the roster as well as the PR list, since whether a reviewer is running
  is half the question and a PR row cannot answer it.
- A reviewer that is up but stuck (blocked, signed out) counts as alive here, and is marked by the
  Agents handle instead. That keeps one situation under one heading rather than two.
