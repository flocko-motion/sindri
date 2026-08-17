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
  no review was requested, one is already claimed, it is an interim milestone whose merge was always
  the user's, or it has left "open". Those are the states that let a PR sit unreviewed while a
  reviewer idled (sd-98fa96) — one call now says which.
- A PR past "open" is reported by the state it is in rather than as one word. Each is a different
  person's move: a rejected PR is live work its author is revising and resubmits, an approved one
  wants your merge, a half-merged one wants inspecting and re-approving. Any command a note names is
  one that succeeds — `pr merge` refuses everything unapproved, so offering it elsewhere would be a
  guaranteed error dressed as advice.
- A reviewer holding a review of a PR that has left "open" is not reported as busy, mirroring the
  release the assignment path performs — a human `pr approve` leaves exactly that state behind, and
  the reviewer's very next ask takes the waiting review.
- A planner and a coauthor are served from no pool, and the answer says so in a sentence rather than
  printing an empty list, which would read as "no work".
- The noun rule holds however the question is asked. `--role` is checked before the call; a named
  agent's role is the hub's to say, so each command checks the answer it got back and names the
  other command rather than printing the other pool.
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
