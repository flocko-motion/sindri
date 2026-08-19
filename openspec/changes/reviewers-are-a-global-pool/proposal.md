# Reviewers are a global pool

## Why

A reviewer belongs to a repo today, and nothing about the role requires it.

The evidence is what a reviewer carries between reviews: nothing. Its assignment lives in the review
row, its verdict is written to the store the moment it is given, and `assignReview` force-checks-out
the PR branch into its worktree — so the tree is replaced per review as surely as the brief is.
`MsgReview` then names the PR, states where the branch is, and lists the three things to do. A
reviewer starts every review from a full brief whatever its session remembers.

So the session context is pure liability, and the repo binding is an accident of how agents are
stored rather than a property of the work.

**The cost is a ceiling that grows with the repo count.** Reviewer RAM is bounded by
*repos × the per-agent limit* — five reviewers exist today across five repos, at 3g each. Sleep and
wake already reclaim the idle ones, so the waste is not idle RAM: it is the peak. Five repos
submitting at once wakes five reviewers, and on a machine with limited headroom that is the
constraint that bites.

**And it is a capability gap, not only a cost.** An idle reviewer in one repo cannot help clear
another repo's review backlog, because `idleReviewer` read one project's roster. No amount of sleep
and wake fixes that, because the constraint is identity rather than power.

## What changes

- A virtual project **`_global`** joins the registry, its path a dedicated directory under the state
  dir (so `repoSlug`/container naming read its name back unchanged, with no special-casing). Every
  existing project-keyed mechanism keeps working unchanged — the store row is `(project, name)`, the
  socket `<state>/_global/sockets/<name>/`, the home `<state>/_global/agents/<name>/`. This is a
  value change where a schema change would be expected.
- Reviewers may live there. A `_global` reviewer serves any project's review queue: `idleReviewer`
  considers it alongside a project's own (preferring local, since a repo that keeps a dedicated
  reviewer expects it used), and `assignReview` resolves its roster row, workspace, state and notes
  from its own home rather than the PR's project — the review record itself stays with the PR's
  project regardless of who is ruling on it.
- Because a `_global` reviewer's own project is never the PR's project, every verb it runs after
  assignment — asking for its own directive, approving, rejecting, showing the diff, running the
  lint gate — resolves the PR's actual project explicitly (the same fallback-then-fleet-scan
  `PRProject` pattern the host-facing HTTP handlers already used) rather than trusting its own.
- **Their workspace is materialised per review** into a fixed path the pod mounts, rather than a git
  worktree inside a repo. A reviewer needs files to read; it has no git of its own (the hub runs a
  curated subset hub-side) and `sindri lint <pr>` executes hub-side too. `git.ArchiveTree` exports
  the PR's tree as plain files (no `.git`), clearing whatever the last review left, so the pod
  becomes repo-agnostic without ever being restarted — a bind mount shows whatever is at its path. A
  `_global` pod's Launch skips the git/worktree machinery entirely (there is no repository to check
  out from), and mounts no worktree or scratch tree — only `/workspace`, exactly as a project-bound
  reviewer's mount table already gave it.
- **The context is cleared after every verdict**, not compacted. A summary keeps the conclusions and
  drops the diff that justified them, which for this role is exactly backwards; and clearing is what
  makes a session safe to point at another repo next.
- `NewAgent` accepts only a reviewer in `_global`, refusing every other role by name — a worker holds
  a branch, a planner a standing conversation, a coauthor the user's own seat, and none of that
  exists in a project with no repo. Both front-ends can create one: the CLI's `agent new --global`
  and the TUI's "reviewer (global)" choice each dial `_global` directly rather than the caller's cwd.
- `_global` agents are **always in scope**, in every repo and both front-ends: the shared
  local-vs-foreign predicate (`inScope` in the TUI, `listGroupFor` in the CLI) admits it
  unconditionally, so it never needs `AgentNeedsUser` to be seen the way a genuinely foreign row
  does. The TUI repo switcher excludes it — not a repo to switch into, and dialing it by its
  registered (hashed) path there would have registered a phantom duplicate.

## What does not change

- **Per-repo reviewers stay possible.** `_global` being an ordinary project means a repo with unusual
  review needs can keep a dedicated reviewer, and the two coexist without a special case.
- Workers, planners and coauthors stay repo-bound. Each holds a branch or the user's own seat, and
  the argument above turns entirely on holding nothing.
- Merge stays human-only, and the review record stays with its project.

## Why `_global`

`RepoTag` is `hex.EncodeToString(sum[:4])` — eight characters from `[0-9a-f]`. `global` contains
`g`, `l` and `o`, so it can never collide with a generated tag. The collision worth guarding is one
layer up: a repo directory literally named `global`, resolved by the CLI's name lookup.

`$global` and `*global` do not survive. `repoSlug` keeps only `[a-z0-9-_]` when building a container
name, so both are silently stripped to `global` — colliding with the very thing they were meant to
distinguish, at the layer that matters. Underscore is in the kept set, so `_global` survives intact
as `sindri-_global-<tag>-<name>`, and is shell-safe unquoted where `*` globs and `$` expands.

The leading underscore earns a second job: wherever a tag is displayed, it reads as *not a repo*.

## Impact

- Code: `internal/hub/workflow/{globalpool,review,reviewhealth,verdict,gate,pr,explain,stall}.go`,
  `internal/hub/{hub,project_config,reqscope,state,commentscope,commands}.go`,
  `internal/hub/project/project.go`, `internal/hub/agent/{lifecycle,boundary,sleep}.go`,
  `internal/hub/store/{workflow,reviewpool}.go`, `internal/adapter/git/git.go` (`ArchiveTree`),
  `internal/api/project.go` (`GlobalProject`), `internal/ui/tui/{items,tab_agents,switcher}.go`,
  `internal/ui/cli/{listing,hub,agent}.go`.
- A `_global` reviewer's own project is never a PR's project, so `reviewDirective`, `CmdApprove`,
  `CmdReject`, `completeReview`, `CmdShowPR`, `CmdLint`'s PR-argument path, `AtLeafBoundary`,
  `HoldsNothing`, `reviewerTasks`, the board's PR column, the stall nudge and `staffHolding` all
  resolve the PR's actual project (or where a held review is filed) explicitly, via the new
  `store.Store.ReviewingPR`/`RuledPRs` (home's own project first, else fleet-wide — safe because an
  agent's name is unique across the whole fleet) and the existing `PRProject` fallback-then-fleet-scan
  for a PR named by id. Agent-identity operations (state, notes, `FireClear`) stay scoped to the
  reviewer's own home throughout. `freeReviewer` also falls back to the pool, so a submit reaches it
  directly rather than only through the periodic `AssignPendingReviews` tick.
