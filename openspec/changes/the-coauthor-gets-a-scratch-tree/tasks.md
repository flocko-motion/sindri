# Tasks

## 1. Hide what it must not touch

- [x] 1.1 The launcher covers the agent-trees directory in a coauthor's `/workspace` with an empty
      read-only directory (`paths.HiddenDir`), created before the pod starts.
- [x] 1.2 The role's mounts move into one pure function, `workspaceMounts`, so the two exceptions to
      isolation sit together — the planner's read-only remount was a `mounts[0] = ...` mutation, and
      the coauthor's rules would have been a second one.

## 2. Give it somewhere to look

- [x] 2.1 A coauthor's launch adds a scratch worktree beside the agent trees, idempotently, so a
      relaunch keeps whatever it was looking at.
- [x] 2.2 It is mounted read-write at `/scratch` — hidden from `/workspace` with the rest of the agent
      trees, so it is reachable only under that name.
- [x] 2.3 `DeleteAgent` removes it. A coauthor's `/workspace` must never be `worktree remove`d, which
      is why the scratch tree needed naming in the delete path rather than falling out of it.

## 3. The verb

- [x] 3.1 `sindri scratch <ref|pr-id> [--force]`, coauthor-only, checks out in place and detached —
      the reviewer's own checkout (`git.CheckoutDetachedClean`), not `MaterializeReview`'s
      remove-and-re-add, which would leave the pod on a deleted inode.
- [x] 3.2 A PR id resolves to its branch, so one operation serves either spelling.
- [x] 3.3 A dirty scratch refuses and names both ways out; `--force` discards deliberately.
- [x] 3.4 No scratch tree (a pod older than this change) says so and names the fix, rather than
      relaying git's words about a directory it never heard of.

## 4. Tell the role

- [x] 4.1 The coauthor's brief and its directive name `/scratch` and the verb, and say why
      `.worktrees/` is empty — unexplained, it reads as a fault rather than as a boundary.

## 5. Pin it

- [x] 5.1 The mounts, per role: a worker and reviewer get one read-write worktree and nothing else; a
      planner's read-only workspace with openspec writable; the coauthor's covered agent trees and its
      scratch tree; and no other role gets a scratch mount.
- [x] 5.2 The verb over a real repo: a PR id lands its author's work detached, a plain ref does the
      same, and the previous checkout's files are gone.
- [x] 5.3 A dirty scratch refuses with the work still there, the refusal names `--force` and
      `/workspace`, and `--force` then discards and checks out.
- [x] 5.4 No ref prints the usage; no scratch tree explains itself.
- [x] 5.5 The registry: `scratch` is the coauthor's and no other role's.

## 6. Correct what the specs claimed

- [x] 6.1 Both modified requirements said `.sindri/` is hidden from an agent, and from a coauthor by an
      empty overlay. Neither is true: `.sindri/config.yaml` is a tracked file of the repo, and the
      hub's own state lives outside every repository, so there is no such mount and never was. The
      requirements now say what is actually shielded.
- [x] 6.2 The two coauthor scenarios keep their `.sindri` TITLES with corrected bodies: a MODIFIED
      block must carry every scenario title the current spec has, so renaming one reads to the
      validator as dropping it.
