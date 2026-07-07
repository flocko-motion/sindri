## 1. The `internal/config` package

- [x] 1.1 Add `internal/config/config.go` with the four-field header; define `Config` (`Architecture`, `Containerfile`, `ReviewPrompt`, `GitHub{Issues bool}`) with `yaml` tags. Import no UI/hub/adapter packages.
- [x] 1.2 Add the `gopkg.in/yaml.v3` dependency (none is vendored today); run `go mod tidy`.
- [x] 1.3 `Load(root string) (Config, error)`: decode global `paths.StateDir()/config.yaml` (if present) as the base, then decode repo `.sindri/config.yaml` on top; decode with `KnownFields(true)` so unknown keys are errors at both layers. Apply defaults (`Architecture` → `ARCHITECTURE.md`) after the overlay.
- [x] 1.4 Validation → error on: malformed YAML, unknown key, wrong-typed value, and any path value that is absolute or escapes the repo root (`filepath.Clean` + prefix check). A **set** path key whose file is absent → error; the default `Architecture` (key unset) is exempt.
- [x] 1.5 Unit tests: no file → defaults, no error; repo-over-global per-key override; each invalid case (bad YAML, unknown key, absolute/escaping path, missing configured file) returns an error; missing-default-ARCHITECTURE is NOT an error.

## 2. Load point and failure scoping

- [x] 2.1 Load via `(*Hub).projectConfig(project)` (thin wrapper over `config.Load`); Launch gates on it and propagates the error, so the triggering operation fails with the config error surfaced.
- [x] 2.2 A bad config in one project fails only that project's operations (config is loaded per project on the resolution path), not the hub or other projects.
- [x] 2.3 The resolved `Config` is read at each consumer's call site (per-project, no hot-reload).

## 3. Architecture-doc path

- [x] 3.1 In `ensureArchitectureDoc`, seed the placeholder ONLY when `cfg.Architecture` is unset (`!cfg.ArchitectureSet`); never write to a project-named path.
- [x] 3.2 Build the reviewer's architecture line from `cfg.Architecture` (`reviewArchitecture(arch)`, threaded through `dirReview`/`msgReview`) instead of a literal.
- [x] 3.3 Test: custom path flows into the reviewer prompt (`TestReviewInstructionsCarryArchitecture`); unset path still seeds+reads `ARCHITECTURE.md` (`TestEnsureArchitectureDoc`); configured-but-missing path errors in `config` validation tests.

## 4. Containerfile path

- [x] 4.1 Thread the resolved `cfg.Containerfile` (absolute) into the image builder as the highest-precedence recipe source via `EnsureImage(root, containerfile, out)`; keep `customDockerfile` discovery as the unset fallback.
- [x] 4.2 The configured recipe's bytes feed the build-key hash and context overlay exactly as a discovered recipe does.
- [x] 4.3 Verified via the threaded `containerfile` param end-to-end (adapters + `EnsureImageWith`); unset preserves current discovery + embedded-default behavior.

## 5. Reviewer prompt path

- [x] 5.1 When `cfg.ReviewPrompt` is set, `ReviewPrompt` builds from that file's contents instead of the default; unset uses the default prompt.
- [x] 5.2 Verified: custom prompt file drives the reviewer directive; unset uses the default.

## 6. GitHub source toggle

- [x] 6.1 `cfg.GitHub.Issues` is defined and validated as the documented hook for `add-github-issue-source` (its task 7.1). That change is unlanded, so there is no consumer yet — the key parses and stores but has no effect today.
- [ ] 6.2 Test: `github.issues: true` enables the source — deferred with the consumer (`add-github-issue-source`).

## 7. Docs & verify

- [x] 7.1 Document `.sindri/config.yaml` and its keys in the README (its own section by the custom-Containerfile note), with an example and the fail-loud behavior.
- [x] 7.2 `make verify` (build + test + lint) green; new package passes the `brokkr` header/comment lint.
- [x] 7.3 `brokkr lint openspec` validates this change's specs.
- [x] 7.4 End-to-end: `architecture:` makes the reviewer read that path; a malformed `.sindri/config.yaml` fails the next agent launch with a clear error naming the file; no config.yaml behaves exactly as before.
