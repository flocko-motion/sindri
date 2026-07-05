# Design — GitHub issues as a third todo source

## Context

`SyncTasks(project)` in `internal/hub/workflow_task.go` is the single point where
sources are merged into the hub's cached `tasks` table: today it pulls td tasks
(via `internal/adapter/td`) and openspec changes (via `internal/adapter/spec`),
appends both as `store.Task` rows, applies hub-side priority overrides, and calls
`ReplaceTasks` (a full-cache atomic replace). Everything downstream reads that
merged cache and distinguishes source only by id prefix (`td-`, `os-`). Adding
GitHub issues means: a new adapter, a third loop in `SyncTasks`, a write-back hook
on merge, and an audit of the id-prefix write-path branches.

## The adapter — `internal/adapter/github`

Follows the established adapter shape: stateless package-level functions, an
explicit `root` argument, no imports from `hub`/`store`/`issue`, a lint-required
four-field header. It is a `gh`-CLI wrapper (like `spec` wraps `openspec` and `td`
wraps `td`), which reuses the user's existing `gh` auth and needs no token
plumbing of our own.

```
Enabled(root string) bool
    // gh present + authenticated + repo has a GitHub remote. Cheap; used to gate.

Issues(ctx, root string) ([]Issue, error)
    // gh issue list --state open --json number,title,body,labels,updatedAt ...
    // Returns []Issue (a dedicated struct, like spec.Change) — NOT issue.Task.
    // gh issue list already excludes PRs.

Close(ctx, root string, number int, comment string) error
    // gh issue close <number> --comment "<comment>"
```

`Issues`/`Close` take a `context.Context` (network-bound, like the `pod` probes)
so the hub can bound the call. The adapter returns its own `Issue{Number, Title,
Body, Labels, UpdatedAt}` type; mapping to `store.Task` happens in the hub, which
keeps the adapter ignorant of the task model.

### Why `gh` CLI, not the REST API directly

Consistency (every other external tool is a CLI adapter), zero auth code (reuse
`gh auth`), and it matches the "optional tool, degrade gracefully" pattern —
`Enabled` is false when `gh` is missing, exactly as `spec.Enabled` is false when
openspec isn't installed.

## Merging into `SyncTasks`

A third loop, gated by the per-project opt-in and `github.Enabled`:

```go
if cfg.GitHubSource && github.Enabled(root) {
    issues, err := github.Issues(ctx, root)
    if err != nil {
        log.Warn(...)          // degrade to absent — do NOT fail the sync
    } else {
        for _, is := range issues {
            rows = append(rows, store.Task{
                ID:       githubID(is.Number),   // "gh-<number>"
                Title:    is.Title,
                Status:   "open",
                Type:     "issue",
                Priority: "P4",                  // lowest tier; overridable
                Description: is.Body,
            })
        }
    }
}
```

`githubID(n int) string` mirrors `specID` but is trivial: `"gh-" + strconv`. The
default `P4` is applied unless the `task_priority` override table already carries a
human re-rating for that id (the existing override merge already does this for
`os-*` rows — reuse it verbatim).

## Throttle

`SyncTasks` can fire every `workPollInterval` (3s) per idle worker. A local td/
openspec read is cheap; a `gh` API call is not, and GitHub rate-limits. Cache the
`Issues` result per project with a short TTL (e.g. 30–60s) — either inside the
adapter (a package-level map keyed by root) or as a small hub-side memo. On a
cache hit within the TTL, `SyncTasks` reuses the last issue list and does no
network call. Explicit user refresh MAY bypass the TTL.

## Write-back on merge

In `workflow_pr.go`, the merge path already closes the linked td task
(`td.Close`). Add a parallel branch: if the merged PR's task id has the `gh-`
prefix, call `github.Close(ctx, root, number, comment)` where the comment notes
the merge (branch + PR). This is best-effort and MUST NOT gate the merge:

- The local merge completes first; the write-back runs after.
- A `github.Close` error is logged/surfaced as a warning, not returned as a merge
  failure.
- If GitHub is unreachable the issue stays open upstream; next human decision.

## Id-prefix write-path audit

Several places branch on `strings.HasPrefix(id, "td-")` to decide whether a
mutation is written back to td or handled hub-side (priority override in
`task_priority`). Each is an implicit "which source owns writes for this id"
decision and must be reviewed for `gh-`:

- `EditTask`, `SetPriority`, `UnassignTask` (`workflow_task.go`) — priority/labels
  for `gh-*` stay hub-side (GitHub has no P-code), same as `os-*`. Status changes
  do NOT push to GitHub except the close-on-merge path above.
- `workflow_pr.go` close path — add the `gh-` close-with-comment branch.
- `hub.go` mock/spec sentinels — unaffected.

The guiding rule: a `gh-*` task's ONLY outbound write to GitHub is close-on-merge.
Everything else (priority, in-progress, branch, PR) stays local — preserving the
offline core of `03-gh-local`.

## Priority rename: trivial → none

`internal/hub/sections.go` currently maps `P4 → "trivial"` and lists
`PriorityWords = [critical, high, mid, low, trivial]`. Rename the P4 word to
`none` in `PriorityLabel`, `PriorityWords`, and keep `trivial` (and `minor`) as
accepted *input* aliases in the reverse map so existing muscle memory and any
stored input still resolve to P4. `internal/tui/theme.go` reads via
`PriorityLabel`, so it follows automatically.

## Config / opt-in

The source is off by default and enabled per project. Reuse whatever per-project
config surface the hub already has (a `.sindri/` setting or project-registry
field); the exact key is an implementation choice, but the behavior is fixed:
absent/false ⇒ no GitHub tasks. This gate exists because importing *all* open
issues into a busy repo's backlog would otherwise be a surprising, disruptive
default.

## Edge cases

- **Issue closed upstream** — only open issues are listed, so `ReplaceTasks` drops
  it on next resync; it leaves the backlog. A worker already holding that `gh-*`
  task keeps its `agent_state`/branch/PR (agent state is not the tasks table), so
  in-flight work is unaffected.
- **Issue re-opened upstream** — reappears with the same `gh-<number>` id; any
  prior hub-side priority override still keyed to that id re-applies.
- **Number collisions** — impossible; GitHub issue numbers are unique per repo and
  we key per project.
- **Rate limit hit despite throttle** — treated like any `Issues` error: warn,
  contribute no tasks this cycle, keep the last cached list until TTL logic
  recovers.
