# A PR must not be created on a base that has already moved

## Why

`CmdSubmit` never compared the branch against the reference. It resolved the base only to record it
on the PR row, so a worker that started an hour ago submitted against the base it branched from, and
the PR was stale the moment it existed — the human then reviews and merges a diff against a state
that is no longer there.

The reason it happens so often is worth naming, because it is not drift the agent could have
noticed. `referenceMoved` rebases every in-flight agent when the reference moves — but a merge the
hub performs itself calls `noteReference`, which records the new tip as already seen precisely so
the hub does not report its own merge as an outside change. `rebasePlanners` then rebases planners
and only planners. So after every hub merge, every worker is silently behind, with nothing having
told it and nothing having moved its tree.

The existing requirement already said the gates run "after the rebase". There is no rebase in the
submit path, so that phrase described something the product did not do.

## What changes

- A submit whose branch is behind its base is refused. No PR is recorded, the worktree is not
  committed, and the agent stays where it was.
- The refusal names how far behind, what arrived, and the two commands that fix it. "Rebase and try
  again" without the reason reads as a ritual; the incoming commits are what let an agent judge
  whether its work still makes sense on top of them.
- The check runs BEFORE the quality gate, not after. Two reasons, and the second is the one the task
  turns on: a gate run on a branch that must rebase is a build and a test suite spent on a result
  nobody keeps; and a rebase performed after the gate would attach to the PR a verdict taken against
  the old base, so the state that actually merges would never have been gated at all. Refusing sends
  the agent back through `rebase` and a fresh `submit`, which re-runs the gates on the tree that will
  land.
- A failed count is not a refusal. The count is the evidence, and blocking a submit on a git command
  that did not answer would strand an agent holding finished work with nothing to fix.

## Scope

- `CmdSubmit`, the path the task names.
- `CmdOpenspec`, the planner's ship verb, which had the identical hole: it resolves the base only to
  record it, and `rebase` is available to planners too. One helper guards both.
- NOT `contribute`, which already rebases onto base and routes conflicts into `sindri resolve`
  (`contribute.go`). It proves the contribution merges rather than assuming it, so the defect is
  simply not there.

## What this does not fix, and does not claim to

A branch under review is deliberately not moved — the reviewer is reading the diff that was
submitted — so a PR can still go stale between submission and merge. The merge path rebases and
routes conflicts back, and `preflight-open-prs` reports on it meanwhile. This closes the window
between finishing the work and putting it up.

And being current is not being green: commits that replay without conflict can still break the
combined result. This makes a PR honest about its base; the gate on the rebased tree is what speaks
to whether it works.

## Impact

- Specs: the `03-gh-local` lint-gate requirement, whose "after the rebase" becomes true.
- Code: `internal/hub/workflow/pr.go` (the guard and both call sites) and `prompts.go` (the reply).
- No new verb: the refusal points at `sindri rebase`, which already resolves conflicts step by step
  and skips commits the base holds.
