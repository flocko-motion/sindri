# Ask what a ROLE would be handed, not only an existing agent

## Why

`task next` answers "what would be assigned next, and why not everything else" — for the backlog, or
for an agent that exists. The useful question is often the other one: would a reviewer have anything
to do if I started one? Is there work a second worker would pick up? Today the only way to find out
is to start one and watch it.

For a reviewer the answer is not a task at all. A reviewer draws from the unclaimed reviews, so the
honest answer is a PR — under a command whose noun is `task`. That is the wrinkle this change had to
settle rather than paper over.

## What changes

- The hub answers for a role as a hypothetical agent of it holding nothing, beside the existing
  agent question. An agent asked about by name is answered for its OWN role, so a reviewer is no
  longer told about a backlog it will never be offered.
- Asking about an agent and a role together is refused. An agent has a role; the two can contradict,
  and choosing a winner would hide that.
- The reviewer's answer lives under `sindri pr next`, and `task next --role reviewer` says so rather
  than answering with PRs under a task noun. The alternative — a new top-level `sindri next` — was
  turned down because every other command in the tree is `<category> <action>`, and one bare verb
  beside them buys a neutral noun at the cost of the shape a user has learnt.
- The "why not" half is kept for every role, since it is the valuable part. For a reviewer that is:
  no review was requested, one is already claimed, the PR has left "open", or it is an interim
  milestone whose merge was always the user's. Those are the states that let a PR sit unreviewed
  while a reviewer idled (sd-98fa96) — one call now says which.
- A planner and a coauthor are served from no pool, and the answer says so in a sentence rather than
  printing an empty list, which would read as "no work".
- The TUI's `n` asks the same question about the tab it is on: the backlog on Tasks, the reviews on
  PRs. Neither front-end derives any of this — the hub answers, both render it with one formatter.

## Impact

- Specs: `05-workflow` (the assignment question, asked of a role).
- Code: `internal/api/next.go`, `internal/hub/workflow/explain.go`, `internal/hub/server.go`,
  `internal/client/client.go`, `internal/ui/theme/task.go`, `internal/ui/cli/task.go`,
  `internal/ui/cli/hub.go`, `internal/ui/tui/onkey.go`, `internal/ui/tui/keys.go`,
  `internal/ui/tui/tab_tasks.go`.
- `NextTask` grows the role rather than gaining a second client method, so the two front-ends cannot
  drift onto different questions.
- The reason column is now padded by display width. It was padded by bytes, and every reason
  containing a dash — most of them — pulled its title two columns left of the row above.
