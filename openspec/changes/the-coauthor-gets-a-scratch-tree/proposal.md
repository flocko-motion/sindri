# The coauthor gets a scratch tree, and the agent trees are hidden from it

## Why

A coauthor's `/workspace` is the user's own checkout — that is what the role IS — and agent worktrees
live at `.worktrees/<name>` INSIDE that checkout. So a coauthor has read, write and execute over every
other agent's live tree, and the hub commits from those trees: an edit made while looking around lands
in another agent's PR as its work, while that agent is still working.

The same fact leaves it with nowhere to look properly. It can read a diff (`sindri show <pr-id>`) but
not build or test the work, because the only trees it can reach are the user's and the ones it must not
touch.

## What changes

- **`.worktrees/` is hidden**, by binding an empty read-only directory over it. Hiding beats
  read-only: a live worktree is the wrong thing to READ as well as to write — it holds work in
  progress, build output and half-finished edits, which say what that agent is doing now, not what its
  PR contains. What a PR contains is `sindri show <pr-id>`, and now `/scratch`.
- **`/scratch` is a second worktree of the coauthor's own**, read-write, created at launch beside the
  agent trees (and hidden with them, so it is reachable only as `/scratch`). Named for what it is:
  disposable, and not where work is meant to live. Removed with the agent, like any other worktree.
- **`sindri scratch <ref|pr-id> [--force]`** asks the hub to check something out into it. A PR id is
  accepted as well as a ref, since "let me look at pr-sd-xxxx" is the common case. Detached, always:
  the branch is held by its author's worktree and git gives a branch to one worktree at a time — and
  inspection wants a commit anyway.
- **A dirty scratch refuses.** A scratch tree is exactly where a half-finished experiment lives and
  nothing in it is recorded anywhere, so a checkout over it is the only warning the agent gets. The
  refusal names both ways forward: move what matters into `/workspace`, or `--force` to discard.
- **The role's mounts now come from one function** (`workspaceMounts`), where the two exceptions to
  agent isolation sit side by side and each says what it buys. The planner's read-only remount was a
  `mounts[0] = ...` mutation halfway down a 200-line launch; the coauthor's rules would have been a
  second one.

## The checkout is the reviewer's, not `pr verify`'s

`MaterializeReview` removes and re-adds the worktree for a fresh checkout, which is right for the
host's disposable review tree and wrong here: the pod holds `/scratch` open, so the container would be
left looking at a deleted inode. The operation reused is the one that puts a PR branch into a
reviewer's LIVE workspace (`git.CheckoutDetachedClean`, as `assignReview` does) — in place, detached,
clean. Same semantics, no second checkout path.

## `git` does not reach out of `/scratch`

A linked worktree's `.git` file points at an absolute host path that the pod does not mount, so git
inside `/scratch` finds no repository — the same reason a worker cannot `git diff` its own tree. The
hub checks out; the coauthor builds and tests. The reply says so, since a writable tree looks like
somewhere git would work.

## What this corrects in the specs

Both requirements being modified say the `.sindri/` directory is hidden from an agent, and a
coauthor's by an empty read-only overlay. Neither is true, and neither has been:

- `.sindri/` in a repo holds that project's own `config.yaml`, a tracked file. Every agent's worktree
  has it, because it is part of the repo — checking it out is not a leak.
- The hub's durable state — the database, the logs, the agent homes — lives outside every repo
  (`paths.StateDir`), so there is nothing in `.sindri/` for an overlay to shield. No such mount
  exists in the launcher, for either role.

The requirements now say what is actually shielded: hub state, by living nowhere near the repo, and
another agent's worktree, by the overmount this change adds. Leaving a false SHALL standing in a
sentence being rewritten anyway would be the worse half of the trade.

## Impact

- Specs: `04-workers`' pod topology (the coauthor has TWO workspaces, which the "and nothing else"
  wording contradicted as written) and `agent-runtime`'s workspace requirement; `agent-runtime` gains
  the verb that fills the scratch tree.
- Code: `internal/hub/workflow/scratch.go` (new), `internal/hub/agent/lifecycle.go`,
  `internal/hub/commands.go`, `internal/tools/paths/paths.go`, and the coauthor's brief and directive
  in `internal/hub/workflow/prompts.go` — a mount an agent is never told about is a mount it never
  uses, and an empty `.worktrees/` with no explanation reads as a fault.
