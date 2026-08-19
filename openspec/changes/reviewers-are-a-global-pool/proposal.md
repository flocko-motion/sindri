# Reviewers are a global pool, not a per-repo fixture

## Why

A reviewer belongs to a repo today, and nothing about the role requires it.

The evidence is what a reviewer carries between reviews: nothing. Its assignment lives in the review
row, its verdict is written to the store the moment it is given, and `assignReview` force-checks-out
the PR branch into its worktree with `CheckoutDetachedClean` — so the tree is replaced per review as
surely as the brief is. `MsgReviewAssigned` then names the PR, states where the branch is, and lists
the three things to do. A reviewer starts every review from a full brief whatever its session
remembers.

So the session context is pure liability, and the repo binding is an accident of how agents are
stored rather than a property of the work.

**The cost is a ceiling that grows with the repo count.** Reviewer RAM is bounded by
*repos × the per-agent limit* — five reviewers exist today across five repos, at 3g each. Sleep and
wake already reclaim the idle ones (four of the five are stopped), so the waste is not idle RAM: it
is the peak. Five repos submitting at once wakes five reviewers, and on a machine reporting
"5 GiB free · fits 1 agents" that is the constraint that bites.

**And it is a capability gap, not only a cost.** `ori` sits stopped with nothing to do and cannot
help `sindri` clear a review backlog, because `idleReviewer` reads one project's roster. One repo's
idle reviewer is unreachable from another's queue. No amount of sleep and wake fixes that, because
the constraint is identity rather than power.

## What changes

- A virtual project **`_global`** joins the registry, its path the state dir. Every existing
  project-keyed mechanism keeps working unchanged — the store row is `(project, name)`, the socket
  `<state>/_global/sockets/<name>/`, the home `<state>/_global/agents/<name>/`. This is a value
  change where a schema change would be expected.
- Reviewers may live there. A `_global` reviewer serves any project's review queue.
- **Their workspace is materialised per review** into a fixed path the pod mounts, rather than a git
  worktree inside a repo. A reviewer needs files to read; it has no git of its own (the hub runs a
  curated subset hub-side) and `sindri lint <pr>` executes hub-side too. So a plain tree suffices,
  and the pod becomes repo-agnostic without ever being restarted — a bind mount shows whatever is at
  its path.
- **The context is cleared after every verdict**, not compacted. A summary keeps the conclusions and
  drops the diff that justified them, which for this role is exactly backwards; and clearing is what
  makes a session safe to point at another repo.
- The reviewer pool becomes cross-project: `idleReviewer` and `AssignPendingReviews` consider
  `_global` reviewers alongside the project's own.
- `_global` agents are **always in scope**, in every repo and both front-ends. They are not foreign
  rows waiting on the user elsewhere; they belong to no repo at all.

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
