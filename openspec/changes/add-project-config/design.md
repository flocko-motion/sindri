# Design — per-project `.sindri/config.yaml`

## Context

`.sindri/` is now purely per-repo config (the hub's own state moved to the home
state dir; `hub/spec.md` requires the hub write no `.sindri/` into a repo). The
one existing customization is a magic-named Containerfile, resolved by
`customDockerfile(projectRoot)` in `internal/container/image.go:219` with the
precedence **repo `.sindri/` → global `paths.StateDir()` → embedded default**.
There is no config-file reader today and no YAML library vendored. This change
generalizes that single-override idea into one declarative file.

## The `internal/config` package

A pure logic package (not an external-tool adapter — it reads a local repo file),
no UI/hub/adapter imports:

```go
type GitHub struct {
    Issues bool `yaml:"issues"`
}

type Config struct {
    Architecture string `yaml:"architecture"`   // repo-relative; default "ARCHITECTURE.md"
    Containerfile string `yaml:"containerfile"`  // repo-relative; "" = discovery fallback
    ReviewPrompt  string `yaml:"review_prompt"`  // repo-relative; "" = default prompt
    GitHub        GitHub `yaml:"github"`
}

// Load reads and validates .sindri/config.yaml for a project, overlaying the
// global config and defaults. Returns an error on ANY invalid config.
func Load(root string) (Config, error)
```

- **YAML**: introduce `gopkg.in/yaml.v3` (well-known, maintained). Decode with
  `KnownFields(true)` so an **unrecognized key is a decode error** — the fail-loud
  requirement for free, at both repo and global layers.
- **Precedence**: decode the global `StateDir()/config.yaml` (if present) into a
  base `Config`, then decode the repo `.sindri/config.yaml` on top. Since YAML
  decode only writes present keys, the repo file overrides per-top-level-key and
  unset keys keep the base/default. Apply defaults (`Architecture` →
  `ARCHITECTURE.md`) after the overlay.
- **Validation** (all → error, none silent):
  - malformed YAML / unknown key (from the decoder).
  - path values (`architecture`, `containerfile`, `review_prompt`) must be
    repo-relative — reject absolute paths and any `..` that escapes root
    (`filepath.Clean` + prefix check against `root`).
  - a **set** path key whose target file is absent → error. The default
    `architecture` (key unset) is exempt — a missing default is seeded, not an
    error (see below).

## Load point and failure scoping

Load once where a project is resolved — `(*Hub).repo(root)` (`hub.go:174`), beside
`RegisterProject` / `ensureArchitectureDoc` / `ensureGitignore`. A `Load` error is
propagated so the operation that triggered the resolution (agent launch, task
sync, image build, detail) fails with the error surfaced to the caller. The hub
serves many projects, so this scopes the failure to the bad project's operations
rather than killing the process — for a single-project coauthor session this is
effectively "crash hard and complain," which is the intent. The error message
names the file and the specific problem.

Because config is read per resolution (like the Containerfile lookup today), no
caching/hot-reload is added; editing config.yaml takes effect on the next
operation that resolves the project.

## Wiring each key

- **`architecture`** — `ensureArchitectureDoc(root)` (`hub.go:180`) seeds the
  placeholder **only when the key is unset** (default `ARCHITECTURE.md`). The
  reviewer injection `reviewArchitecture` (`prompts.go:23`), currently the literal
  `" Read /workspace/ARCHITECTURE.md now …"`, is built from the configured path:
  `" Read /workspace/<architecture> now …"`. `/workspace` is the mounted repo
  root, so the setting is just the repo-relative path interpolated in.
- **`containerfile`** — thread the resolved absolute path into the image builder
  as the highest-precedence source. `customDockerfile` (`image.go:219`) keeps its
  discovery for the unset case; when set, that path is used directly (and its bytes
  still feed the build-key hash / overlay exactly as a discovered recipe does).
- **`review_prompt`** — when set, the reviewer prompt assembled in
  `internal/hub/prompts.go` / `claude.go` is built from the file's contents instead
  of the default. (Complements today's editable default; the config key makes the
  source explicit and repo-committed.)
- **`github.issues`** — the task-sync gate in `add-github-issue-source` (its
  `design.md` sketch `if cfg.GitHubSource && github.Enabled(root)`) reads this
  bool. This change provides `cfg`; that change consumes it (its task 7.1).

## Sequencing with add-github-issue-source

`add-github-issue-source` is merged but not yet implemented/archived, so its
`github-issues` capability is not in `specs/` yet — this change therefore does not
`MODIFY` it; it defines the `github.issues` config key in the new `project-config`
capability and cross-references. Implementation order: land `internal/config`
first (this change), then that change's task 7 reads `cfg.GitHub.Issues` rather
than inventing its own surface. If this change lands second, that change's 7.1
becomes a one-line hook into `config.Load`.

## Alternatives considered

- **Per-project columns on the `projects` registry** — rejected: the registry is
  keyed by a path hash and auto-populated, with no natural user-editing surface. A
  repo-committed file is the "project config" fit and matches the Containerfile.
- **Lenient/ignore-unknown parsing** — rejected: contradicts the explicit
  fail-loud requirement. `KnownFields(true)` is deliberate. Trade-off: a newer
  config key read by an older sindri errors rather than being ignored; acceptable
  for a locally-read project file (not a wire format), and the loud failure is the
  desired behavior.
