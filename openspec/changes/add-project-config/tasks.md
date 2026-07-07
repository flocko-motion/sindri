## 1. The `internal/config` package

- [ ] 1.1 Add `internal/config/config.go` with the four-field header; define `Config` (`Architecture`, `Containerfile`, `ReviewPrompt`, `GitHub{Issues bool}`) with `yaml` tags. Import no UI/hub/adapter packages.
- [ ] 1.2 Add the `gopkg.in/yaml.v3` dependency (none is vendored today); run `go mod tidy`.
- [ ] 1.3 `Load(root string) (Config, error)`: decode global `paths.StateDir()/config.yaml` (if present) as the base, then decode repo `.sindri/config.yaml` on top; decode with `KnownFields(true)` so unknown keys are errors at both layers. Apply defaults (`Architecture` → `ARCHITECTURE.md`) after the overlay.
- [ ] 1.4 Validation → error on: malformed YAML, unknown key, wrong-typed value, and any path value that is absolute or escapes the repo root (`filepath.Clean` + prefix check). A **set** path key whose file is absent → error; the default `Architecture` (key unset) is exempt.
- [ ] 1.5 Unit tests: no file → defaults, no error; repo-over-global per-key override; each invalid case (bad YAML, unknown key, absolute/escaping path, missing configured file) returns an error; missing-default-ARCHITECTURE is NOT an error.

## 2. Load point and failure scoping

- [ ] 2.1 Call `config.Load(root)` in `(*Hub).repo(root)` (`hub.go:174`); propagate the error so the triggering operation (agent launch, task sync, image build, detail) fails with the config error surfaced.
- [ ] 2.2 Ensure a bad config in one project fails only that project's operations, not the hub or other projects (test with two projects, one with invalid config).
- [ ] 2.3 Make the resolved `Config` reachable where each consumer needs it (thread through, or memo per project on the resolution path — no hot-reload).

## 3. Architecture-doc path

- [ ] 3.1 In `ensureArchitectureDoc` (`hub.go:180`), seed the placeholder ONLY when `cfg.Architecture` is the default/unset; never write to a project-named path.
- [ ] 3.2 Build the reviewer's architecture line from `cfg.Architecture` instead of the literal in `prompts.go:23` (`Read /workspace/<architecture> now …`).
- [ ] 3.3 Test: custom path appears in the reviewer prompt; unset path still seeds+reads `ARCHITECTURE.md`; configured-but-missing path errors (no seed).

## 4. Containerfile path

- [ ] 4.1 Thread the resolved `cfg.Containerfile` (absolute) into the image builder as the highest-precedence recipe source; keep `customDockerfile` discovery (`image.go:219`) as the unset fallback.
- [ ] 4.2 Ensure the configured recipe's bytes feed the build-key hash and context overlay exactly as a discovered recipe does.
- [ ] 4.3 Test: explicit `containerfile` path wins over `.sindri/Containerfile` discovery; unset preserves current discovery + embedded-default behavior.

## 5. Reviewer prompt path

- [ ] 5.1 When `cfg.ReviewPrompt` is set, build the reviewer prompt from that file's contents instead of the default; unset uses the default prompt.
- [ ] 5.2 Test: custom prompt file drives the reviewer directive; unset uses the default.

## 6. GitHub source toggle

- [ ] 6.1 Expose `cfg.GitHub.Issues` to the task-sync path; the `add-github-issue-source` gate reads it (its task 7.1). If that change is unlanded, leave a documented hook so it wires in cleanly.
- [ ] 6.2 Test: `github.issues: true` enables the source (with `gh` faked available); unset/false imports nothing.

## 7. Docs & verify

- [ ] 7.1 Document `.sindri/config.yaml` and its keys in the README (near the custom-Containerfile note), with an example and the fail-loud behavior.
- [ ] 7.2 `make verify` (build + test + lint) green; new package passes the `brokkr` header/comment lint.
- [ ] 7.3 `brokkr lint openspec` validates this change's specs.
- [ ] 7.4 End-to-end: a repo with `architecture: openspec/architecture.md` makes the reviewer read that path; a malformed `.sindri/config.yaml` makes the next agent launch fail with a clear error naming the file; no config.yaml behaves exactly as before.
