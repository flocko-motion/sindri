## 1. The `internal/adapter/github` package

- [x] 1.1 Add `internal/adapter/github/github.go` with the four-field header (`type: adapter (external tool)`), importing nothing from `hub`/`store`/`issue`.
- [x] 1.2 `Enabled(root string) bool` — cheap, local, side-effect-free: `gh` on PATH + repo has a GitHub remote. Do NOT probe the network for auth here; an unauthenticated/offline `gh` is handled at call time by `Issues()` erroring and `SyncTasks` degrading to no tasks.
- [x] 1.3 `Issues(ctx, root string) ([]Issue, error)` via `gh issue list --state open --limit <high> --json number,title,body,labels,updatedAt` — MUST pass an explicit high `--limit` (e.g. 1000), or paginate, because `gh issue list` defaults to `--limit 30` and would silently drop the rest. Define the `Issue` struct (`Number, Title, Body, Labels, UpdatedAt`). Confirm PRs are excluded.
- [x] 1.4 `Close(ctx, root string, number int, comment string) error` via `gh issue close <number> --comment <comment>`.
- [x] 1.5 Unit tests with a faked `gh` (stub binary on PATH or exec seam): enabled/disabled detection, list parsing, close command shape, error surfacing, and **a >30-issue list** that asserts every issue is returned (guards the `--limit` fix).

## 2. Merge issues into the cache

- [x] 2.1 Add `githubID(number int) string` (`gh-<number>`) in `internal/hub/github_source.go` (kept out of `workflow_task.go` for the loc gate).
- [x] 2.2 Add the third source loop in `SyncTasks`: when opt-in is on and `github.Enabled(root)`, append each issue as `store.Task{ID: githubID, Type: "issue", Status: "open", Priority: "P4", Title, Description}`. On `Issues` error, log a warning and contribute no tasks — never fail the sync.
- [x] 2.3 Confirm the existing `task_priority` override merge applies to `gh-*` rows exactly as it does to `os-*` (default P4 unless a human re-rating exists).
- [x] 2.4 Verify `gh-*` rows are childless leaves and are returned by `OpenLeaves()` so auto-assignment claims them (asserted by `TestOpenLeavesExcludesHeldLeaf`).

## 3. Throttle the network source

- [x] 3.1 Cache the `Issues` result per project with a short TTL (45s) so the 3s idle-worker resync does not hit GitHub every cycle; on a cache hit, do no network call (hub-side memo in `github_source.go`; the adapter stays stateless).
- [x] 3.2 A `force` refresh bypasses the TTL, and a fetch error keeps the last good list until it recovers (never blanks the backlog).

## 4. Close-and-comment on merge

- [x] 4.1 In the merge path (`workflow_merge.go`), add a `gh-` branch: after the local merge, call `github.Close(ctx, root, number, "merged by sindri: <branch> (<pr>)")`.
- [x] 4.2 Best-effort: runs after the merge, never blocks or fails it on a GitHub error; the failure is logged and recorded on the PR as a `warning`.

## 5. Id-prefix write-path audit

- [x] 5.1 Audited every `strings.HasPrefix(id, "td-")` branch (`workflow_task.go`: `EditTask`, `SetPriority`, `UnassignTask`, `CloseTask`, `claimLeaf`; `workflow_merge.go`; `hub.go`). `gh-*` correctly falls through to hub-side handling everywhere; guarded `claimLeaf` so it only flips td status for `td-` ids (a `gh-`/`os-` id would error), and added the `agent_state` held-leaf exclusion to `OpenLeaves` so a claimed issue isn't re-handed-out.
- [x] 5.2 Enforced: a `gh-*` task's only outbound GitHub write is close-on-merge — priority/labels/in-progress stay hub-side. `TestGitHubTaskPriorityStaysHubSide` covers `SetPriority`/`EditTask` recording a hub-side override, never touching td or GitHub.

## 6. Priority rename: trivial → none

- [x] 6.1 In `internal/hub/sections.go`, renamed the P4 display word `trivial` → `none` in `PriorityLabel` and `PriorityWords`.
- [x] 6.2 Kept `trivial` (and `minor`) as accepted input aliases in `PriorityCode` so existing input still resolves to P4.
- [x] 6.3 `internal/tui/theme.go` reads via `PriorityLabel` (follows automatically); updated the CLI `priority` command's word enumeration in `cmd/sindri/task.go` to `none`.

## 7. Opt-in config

- [x] 7.1 The per-project opt-in is the `github.issues` key in `.sindri/config.yaml` (off by default), on the config surface added by `add-project-config`.
- [x] 7.2 `SyncTasks` reads the toggle (`cfg.GitHub.Issues`); when off, no issues are imported regardless of `gh` availability.

## 8. Verify

- [x] 8.1 `make verify` (build + test + lint) green; new adapter passes the `brokkr` comments/header lint.
- [x] 8.2 `brokkr lint openspec` validates this change's specs.
- [ ] 8.3 End-to-end in a repo with a GitHub remote and the source enabled: an open issue appears as `gh-<n>` at priority `none`, a worker claims and PRs it, and on merge the issue is closed+commented on GitHub. (Requires a live GitHub repo + auth — run manually.)
- [x] 8.4 Degradation check: with the source enabled but `gh` unavailable/offline, sync completes with no GitHub tasks and no error (`Enabled`/`Issues`-error paths degrade to nil); a `gh-*` merge with GitHub unreachable still merges locally and warns (close-on-merge is best-effort by construction).
