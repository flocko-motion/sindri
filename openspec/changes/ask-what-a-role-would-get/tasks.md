# Tasks

## 1. Answer for a role

- [x] 1.1 `ExplainNext` takes a role and answers as a hypothetical agent of it holding nothing.
- [x] 1.2 A named agent is answered for its own role; an unknown role is refused; an agent and a
      role together are refused.
- [x] 1.3 The reviewer's pick comes from `UnclaimedReview` — the query the directive hands out
      from — so the explanation cannot drift from the decision.

## 2. Keep the "why not" half for every role

- [x] 2.1 Each open PR carries its standing: unclaimed, claimed (and by whom), unrequested, interim,
      or no longer open — with what would move it on.
- [x] 2.2 A planner and a coauthor say how they are given work instead, rather than listing nothing.

## 3. Put the reviewer answer under a PR noun

- [x] 3.1 `sindri pr next` answers for a reviewer, with `--agent` for one that exists.
- [x] 3.2 `task next --role reviewer` names that command instead of answering with PRs.
- [x] 3.3 The TUI's `n` asks about the tab's own pool — the backlog on Tasks, the reviews on PRs.

## 4. Pin it

- [x] 4.1 A reviewer is answered with none running, and the task half stays empty for it.
- [x] 4.2 Every reason a PR goes unreviewed, including a merged one being off the board entirely.
- [x] 4.3 The refusals: agent-with-role, an unknown role, and each front-end pointer.
- [x] 4.4 A reviewer holding a review takes nothing, as a worker holding a task does.
