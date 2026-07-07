# Add a per-project `.sindri/config.yaml`

## Why

Now that the hub is centralized in the user's home directory, a repo's `.sindri/`
is no longer runtime state — it is purely **project-specific config**. Today the
only thing a project can customize there is a magic-named `Containerfile`. Yet
several behaviors are hard-coded that a project would reasonably want to set:

- The **architecture doc** the reviewer is told to read is the literal
  `/workspace/ARCHITECTURE.md` (`prompts.go:23`) — but a project may keep its
  architecture under `openspec/`, or name it differently.
- The **custom image recipe** and the **reviewer prompt** are discovered by magic
  filenames (`.sindri/Containerfile`, `.sindri/review-prompt.txt`) with no single
  place that declares them.
- The just-merged **GitHub issue source** (`add-github-issue-source`) explicitly
  deferred its per-project opt-in toggle to "whatever config surface exists" —
  and there is none yet.

One declarative `.sindri/config.yaml` gives projects a single, discoverable place
to set these, and gives the GitHub-source toggle a real home.

## What Changes

- **New `.sindri/config.yaml`**, read from the repo root (the same directory the
  custom Containerfile already lives in). A repo config overlays an optional
  global `config.yaml` under the hub's state dir, which overlays built-in
  defaults — mirroring the existing Containerfile precedence.
- **New `internal/config` package** — a pure loader/validator (`Load(root)
  (Config, error)`), no UI or adapter dependencies. Introduces a YAML dependency
  (none is vendored today).
- **Four v1 keys:**
  - `architecture` — repo-relative path to the architecture doc the reviewer is
    pointed at (default `ARCHITECTURE.md`).
  - `containerfile` — repo-relative path to the agent image recipe (overrides the
    magic-filename discovery).
  - `review_prompt` — repo-relative path to a file whose contents replace the
    default reviewer prompt.
  - `github.issues` — bool; enables the GitHub issue source for this project
    (default `false`). This is the toggle `add-github-issue-source` deferred.
- **Fail loud — invalid config is never silently accepted.** A malformed file, an
  unknown key, a wrong-typed value, or a referenced path that is absolute,
  escapes the repo, or does not exist SHALL cause the operation that needs the
  project (launching an agent, syncing tasks, building the image) to fail with a
  clear, actionable error naming the file and the problem. The hub SHALL NOT fall
  back to defaults or ignore a bad config.
- **Absent config is not an error.** With no `.sindri/config.yaml`, every behavior
  keeps its current default (reviewer reads `ARCHITECTURE.md`, filename discovery
  finds the Containerfile, the GitHub source stays off) — fully backward
  compatible.
- **Architecture-doc seeding narrows.** The placeholder `ARCHITECTURE.md` is still
  auto-seeded only in the default case (no `architecture` key set). When
  `architecture` names a path, that file MUST exist — a missing configured doc is
  invalid config and fails loudly; the hub never writes to a path the project
  named.

## Capabilities

### New Capabilities
- `project-config`: a per-project `.sindri/config.yaml` that declares a project's
  architecture-doc path, image recipe, reviewer prompt, and GitHub-source toggle;
  resolved repo-over-global-over-default; validated fail-loud so an invalid config
  is surfaced, never silently ignored.

### Modified Capabilities
<!-- none. The GitHub-source toggle is defined here as the config surface and
     cross-references the `github-issues` capability from add-github-issue-source
     (not yet archived into specs/), so no existing spec is modified. -->

## Impact

- **New package** `internal/config` (+ tests): `Config` struct, `Load(root)`,
  validation, repo/global/default precedence. New YAML dependency (e.g.
  `gopkg.in/yaml.v3`).
- **`internal/hub/prompts.go` / `internal/hub/hub.go`**: the reviewer's
  architecture line (`prompts.go:23`) and the seed target (`hub.go:186`) become
  the configured path instead of the literal `ARCHITECTURE.md`; seeding runs only
  when the key is unset.
- **`internal/hub/claude.go` / reviewer prompt assembly**: when `review_prompt` is
  set, its contents replace the default reviewer prompt.
- **`internal/container/image.go`**: `customDockerfile` gains the configured
  `containerfile` path as the highest-precedence source; magic-filename discovery
  remains the fallback.
- **`internal/hub` task sync**: reads `github.issues` to gate the GitHub source
  (satisfies `add-github-issue-source` task 7.1).
- **Config load point**: alongside `RegisterProject`/`ensureArchitectureDoc` in
  `(*Hub).repo(root)`, so a bad config fails the affected project's operations
  (not the whole hub, which serves other projects).
- **No change to the wire format or hub DB schema.** Config lives in the repo, not
  the registry.

## Non-goals

- Broadening who reads the architecture doc — it stays a **reviewer**-only prompt
  (workers/planners are unchanged). A worker-facing architecture brief is a
  possible follow-up.
- Any GitHub-source behavior beyond the on/off toggle (scope, priority, write-back
  are all fixed by `add-github-issue-source`).
- Migrating existing magic-filename overrides — they keep working as the fallback;
  config.yaml is additive.
- Per-key hot-reload — config is read when a project is resolved, like today's
  Containerfile lookup.
