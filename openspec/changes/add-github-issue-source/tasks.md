## 1. The `internal/adapter/github` package

- [ ] 1.1 Add `internal/adapter/github/github.go` with the four-field header (`type: adapter (external tool)`), importing nothing from `hub`/`store`/`issue`.
- [ ] 1.2 `Enabled(root string) bool` — true only when `gh` is on PATH, authenticated, and the repo has a GitHub remote; cheap and side-effect-free.
- [ ] 1.3 `Issues(ctx, root string) ([]Issue, error)` via `gh issue list --state open --json number,title,body,labels,updatedAt`; define the `Issue` struct (`Number, Title, Body, Labels, UpdatedAt`). Confirm PRs are excluded.
- [ ] 1.4 `Close(ctx, root string, number int, comment string) error` via `gh issue close <number> --comment <comment>`.
- [ ] 1.5 Unit tests with a faked `gh` (stub binary on PATH or exec seam): enabled/disabled detection, list parsing, close command shape, and error surfacing.

## 2. Merge issues into the cache

- [ ] 2.1 Add `githubID(number int) string` (`gh-<number>`) in `internal/hub/workflow_task.go`.
- [ ] 2.2 Add the third source loop in `SyncTasks`: when opt-in is on and `github.Enabled(root)`, append each issue as `store.Task{ID: githubID, Type: "issue", Status: "open", Priority: "P4", Title, Description}`. On `Issues` error, log a warning and contribute no tasks — never fail the sync.
- [ ] 2.3 Confirm the existing `task_priority` override merge applies to `gh-*` rows exactly as it does to `os-*` (default P4 unless a human re-rating exists).
- [ ] 2.4 Verify `gh-*` rows are childless leaves and are returned by `OpenLeaves()` so auto-assignment claims them (no code change expected — assert with a test).

## 3. Throttle the network source

- [ ] 3.1 Cache the `Issues` result per project with a short TTL (30–60s) so the 3s idle-worker resync does not hit GitHub every cycle; on a cache hit, do no network call.
- [ ] 3.2 Allow explicit user refresh to bypass the TTL (optional); ensure a stale error keeps the last good list until it recovers.

## 4. Close-and-comment on merge

- [ ] 4.1 In `internal/hub/workflow_pr.go` merge path, add a `gh-` branch: after the local merge, call `github.Close(ctx, root, number, "merged by sindri: <branch>/<pr>")`.
- [ ] 4.2 Make it best-effort: run after the merge, never block or fail the merge on a GitHub error; surface the failure as a warning notification.

## 5. Id-prefix write-path audit

- [ ] 5.1 Audit every `strings.HasPrefix(id, "td-")` branch (`workflow_task.go`: `EditTask`, `SetPriority`, `UnassignTask`; `workflow_pr.go`; `hub.go`) and add `gh-` handling where needed.
- [ ] 5.2 Enforce the rule: a `gh-*` task's only outbound GitHub write is close-on-merge — priority/labels/in-progress all stay hub-side. Add tests covering that a `SetPriority`/`EditTask` on a `gh-*` task does NOT call the github adapter.

## 6. Priority rename: trivial → none

- [ ] 6.1 In `internal/hub/sections.go`, rename the P4 display word `trivial` → `none` in `PriorityLabel` and `PriorityWords`.
- [ ] 6.2 Keep `trivial` (and `minor`) as accepted input aliases in the reverse map so existing input still resolves to P4.
- [ ] 6.3 Confirm `internal/tui/theme.go` and any CLI help/text that reads `PriorityLabel` now show `none`; update the `--priority` flag help string in `cmd/sindri/hub.go` if it enumerates the words.

## 7. Opt-in config

- [ ] 7.1 Add a per-project opt-in toggle for the GitHub source (off by default) on the hub's existing config/registry surface.
- [ ] 7.2 `SyncTasks` reads the toggle; when off, no issues are imported regardless of `gh` availability.

## 8. Verify

- [ ] 8.1 `make verify` (build + test + lint) green; new adapter passes the `brokkr` comments/header lint.
- [ ] 8.2 `brokkr lint openspec` validates this change's specs.
- [ ] 8.3 End-to-end in a repo with a GitHub remote and the source enabled: an open issue appears as `gh-<n>` at priority `none`, a worker claims and PRs it, and on merge the issue is closed+commented on GitHub.
- [ ] 8.4 Degradation check: with the source enabled but `gh` unavailable/offline, sync completes with no GitHub tasks and no error; a `gh-*` merge with GitHub unreachable still merges locally and warns.
