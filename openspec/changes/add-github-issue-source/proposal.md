# Add GitHub issues as a third todo source

## Why

Sindri already draws work from two sources — `td` tasks and openspec changes —
merged into one cached backlog the hub hands out to workers. But a lot of real
work already lives as **GitHub issues**, and today none of it reaches an agent
without a human first re-typing it as a td task. We want issues to flow straight
into the same backlog: open an issue on GitHub, and a worker can pick it up,
build it, and — on merge — close the issue back on GitHub. A third source, same
loop.

This is deliberately narrow. Sindri's PR/merge workflow stays **local-only** as
`03-gh-local` requires: we are not moving pull requests to GitHub. GitHub is
touched for exactly two things — *reading* open issues inbound, and *closing +
commenting* an issue when its local PR merges. Everything else — branches, PRs,
review, the human merge gate — remains offline git, untouched.

## What Changes

- **New `internal/adapter/github` package** — a stateless, `gh`-CLI-backed adapter
  (mirroring how `spec` and `td` shell out, reusing the user's existing `gh`
  auth). It exposes: detect/enable, list open issues, and close-with-comment. It
  imports nothing from `hub`/`store`/`issue`.
- **Issues become cached tasks.** `SyncTasks` gains a third loop appending each
  open issue as a `store.Task` with a `gh-<number>` id (the issue number, stable
  and human-meaningful) and type `issue`. They are childless leaves, so the
  existing auto-assignment claims them exactly like any other leaf — **directly
  claimable, no gate.**
- **Import scope is all open issues.** Every open issue in the repo's GitHub
  remote is imported (pull requests are excluded — `gh issue list` already does).
- **Close + comment on merge.** When a worker's PR for a `gh-*` task merges, the
  hub closes the GitHub issue and leaves a comment noting the merge. This is
  best-effort: if GitHub is unreachable the *local* merge still succeeds and the
  write-back is skipped with a warning — the merge is never blocked on the network.
- **Priority: default to the lowest tier.** GitHub issues carry no native
  priority, so a newly imported issue defaults to the lowest priority tier and a
  human re-rates it via the existing hub-side priority override (the same
  mechanism openspec `os-*` items already use). As part of this we **rename the
  lowest priority tier's display word from `trivial` to `none`** — the honest
  label for "came in unrated."
- **Opt-in per project, graceful when absent.** The GitHub source is **off by
  default** and enabled per project; importing *all* open issues into a busy
  repo's backlog should never be a surprise. And because it is a network source,
  it degrades to absent — never a hard failure — whenever `gh` is missing, the
  repo has no GitHub remote, the user is unauthenticated, or the network is down
  (the same "optional source" posture as openspec).

## Capabilities

### New Capabilities
- `github-issues`: importing open GitHub issues as claimable todos through a
  `gh`-backed adapter, minting stable `gh-<number>` ids, defaulting them to the
  lowest priority tier, closing+commenting the issue on merge, and degrading to
  absent when GitHub is unreachable. Opt-in per project.

### Modified Capabilities
- `03-gh-local`: scope the "self-contained, no remote dependency" guarantee to the
  PR/worktree/merge workflow explicitly, and record that the *issue source* is a
  separate, optional network integration whose absence never breaks the offline
  core.
- `hub`: the cached read model now merges more than one source; network sources
  SHALL be throttled so the frequent (every-few-seconds) idle-worker resync does
  not hammer the GitHub API.

## Impact

- **New package**: `internal/adapter/github` (+ tests). `gh`-CLI-backed;
  `Enabled(root)`, `Issues(root)`, `Close(root, number, comment)`.
- **`internal/hub/workflow_task.go`**: a `githubID(n)` helper (`gh-<number>`), a
  third source loop in `SyncTasks`, and a default-priority assignment for new
  `gh-*` rows; audit the `strings.HasPrefix(id, "td-")` write-path checks and add
  `gh-` handling where a mutation should reach GitHub vs. stay hub-side.
- **`internal/hub/workflow_pr.go`**: on merge of a `gh-*` task, call the adapter's
  close-with-comment (best-effort, non-blocking).
- **`internal/hub/sections.go`**: rename the P4 display word `trivial` → `none`
  (in `PriorityLabel`, the reverse map, and `PriorityWords`); keep `trivial`
  accepted as an input alias for back-compat.
- **Config**: a per-project opt-in toggle to enable the GitHub source.
- **Throttle**: a min-interval cache around the `gh issue list` call so the 3s
  idle-worker resync doesn't exceed GitHub rate limits.
- **No change to the PR/merge machinery, the wire format, or data migrations.**
  `gh-*` rows ride the existing `store.Task` schema and board path; UIs render
  them with no changes required (a distinct glyph is a possible follow-up).

## Non-goals

- Moving PRs, review, or merge to GitHub — the local-only workflow stands.
- Mapping GitHub labels/milestones/projects to priority or type (issues come in
  as `none`; re-rate by hand). A label→priority mapping is a possible follow-up.
- Writing sindri's in-progress/working state back to GitHub — only close-on-merge
  writes back; the working phase stays hub-side and offline-safe.
- Two-way sync of issue *edits* (title/body changes flow inbound on resync; sindri
  does not push edits outbound).
