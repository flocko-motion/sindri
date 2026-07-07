## 1. Registry store surface

- [x] 1.1 Add a `last_used` column to the `projects` table (idempotent `ALTER` in `migrate()`); extend `store.Project` with `LastUsed`. Touch it on `RegisterProject` (and on project resolution) so recency ordering has a signal.
- [x] 1.2 Add `Store.UnregisterProject(tag string) error` — deletes only the `projects` row for the tag; touches no other table and no files.
- [x] 1.3 Confirm `Store.Projects()` returns the fields the switcher/`repo list` need (tag, path, first_seen, last_used); add a live-agent count join or compute it in the hub read.
- [x] 1.4 Store tests: register updates `last_used`; unregister removes the row and nothing else; `Projects()` carries `last_used`.

## 2. Hub read/write surface

- [x] 2.1 `RepoList()` — every registered repo with path, live-agent count, and config/source flags (reads registry + roster; live-agent count reuses the roster/liveness path).
- [x] 2.2 `RepoInfo(project string)` — one repo's resolved `config.Config` plus its agent/PR/task counts.
- [x] 2.3 `RepoInit(root string)` — idempotent: `RegisterProject`, scaffold `<root>/.sindri/config.yaml` from a commented template only if absent (never overwrite), seed `ARCHITECTURE.md` via `ensureArchitectureDoc` when the project has no configured `architecture`.
- [x] 2.4 `RepoForget(project string)` — delete the repo's agents (via `DeleteAgent`, freeing pods/worktrees/identities), then `UnregisterProject`; never touch `.sindri/`, git, or the repo's passive records (task cache, priority overrides, approvals, PRs, events) — those stay keyed by the stable tag so re-adding reactivates them. Scope the board's global PR list to registered projects so forgotten records don't surface until re-added.
- [x] 2.5 `WriteRepoConfig(root string, cfg config.Config)` — serialize to `.sindri/config.yaml` after validating via the config package's load path; return the validation error rather than persisting a broken config.
- [x] 2.6 Wire these onto the hub HTTP/JSON surface (new registry endpoints) and the client used by CLI + TUI.

## 3. CLI `repo` command set

- [x] 3.1 Add `cmd/sindri/repo.go` with `repo init | list | info | forget`, defaulting to cwd's git root like other commands; bare `repo` prints `info` for cwd.
- [x] 3.2 `repo list` renders a table (repo, path, #agents, source flags); `repo info [repo]` renders resolved config + counts; `repo forget <repo>` surfaces the agent-guard message on refusal.
- [x] 3.3 CLI tests: `init` scaffolds config + registers and is idempotent; `forget` refuses with live agents and succeeds when idle; `list`/`info` shape.

## 4. TUI — persistent indicator + scalable switcher

- [x] 4.1 Show the active repo's name persistently in the top bar, in that repo's deterministic color (reuse the project color scheme).
- [~] 4.2 Ordering done (live-agents-first → recency → alphabetical) + it's already a modal, not a tab strip; scrollable/typeahead-filter still TODO.
- [x] 4.3 Selecting a repo updates the top-bar indicator and rescopes the Tasks tab.

## 5. TUI — global/repo scope toggle

- [x] 5.1 Add a scope toggle (a key, e.g. `g`) to the Agents and PRs tabs, `global ↔ repo`, default `global`; show the active scope in the footer.
- [x] 5.2 In `repo` mode, filter the (already-global) `BoardState.Agents`/`.PRs` to the active repo — a pure view filter, no new hub call.
- [x] 5.3 Confirm the Tasks tab stays always-scoped to the active repo regardless of the other tabs' toggle.

## 6. TUI — repo config form

- [x] 6.1 Add a config-edit form over the `.sindri/config.yaml` keys (`architecture`, `containerfile`, `review_prompt`, `github.issues`) reusing the task-form field primitives.
- [x] 6.2 Save through `WriteRepoConfig`; on a validation error, show it in the form and do not persist.

## 7. Verify

- [x] 7.1 `make verify` (build + test + lint) green; new files pass the `brokkr` header/comment/loc lint.
- [x] 7.2 `brokkr lint openspec` validates this change's specs.
- [~] 7.3 (manual) End-to-end: `repo init` in a fresh repo scaffolds config + registers; it appears in the switcher and top bar; the Agents/PRs global↔repo toggle narrows/expands; editing config via the form takes effect; `repo forget` on an idle repo drops it (files intact) and it reappears on next use.
- [x] 7.4 Confirm the non-goal holds: no `Project` field was added to `store.Task`, no global `AllTasks`, and the task tree is unchanged.
