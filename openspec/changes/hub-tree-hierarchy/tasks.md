## 1. `hub/world/` — what the hub knows (commit 1)

- [x] 1.1 Move `hub/observe` → `hub/world/observe` and rewrite its importers
- [x] 1.2 Move `hub/store` → `hub/world/store` and rewrite its importers (100+ files)
- [x] 1.3 Move `hub/task` → `hub/world/task` and rewrite its importers
- [x] 1.4 Move `hub/owned` → `hub/world/owned` and rewrite its importers
- [x] 1.5 Move `hub/situation` → `hub/world/situation` and rewrite its importers
- [x] 1.6 Update the `// package:` header in every moved file (`brokkr lint header-path` gates it)
- [x] 1.7 Point `flowMayImport` at the new `world/store`, `world/situation` and `world/task` paths
- [x] 1.8 Point `internal/arch/situation_test.go` at `internal/hub/world/situation`, so the
      no-runtime-call guard keeps applying, and confirm it still fails when given a querying import
- [x] 1.9 Point every other arch allowlist naming a moved path at its new one
- [x] 1.10 Green: `go build ./...`, the full suite, all 10 linters

## 2. `hub/flow/prompts` — attempted, reverted (see design.md)

- [x] 2.1 Add `vocabularyDirs` to `internal/arch/flow_test.go`, skipped by the purity walk beside
      the existing `isActing`
- [x] 2.2 Prove it: give a state map a writing import, watch the guard still fail, restore
- [x] 2.3 Move `hub/prompts` → `hub/flow/prompts` and rewrite its 33 importers
- [x] 2.4 Update the moved files' `// package:` headers
- [x] 2.5 Confirm `prompts` stays OFF `flowMayImport`, so a state map still cannot inline prose
- [x] 2.6 Green: `go build ./...`, the full suite, all 10 linters

## 3. `hub/sweep/` — attempted, reverted (see design.md)

- [x] 3.1 Declare `sweep.Deps` in `internal/hub/sweep`: the fifteen things the sweeps and the
      watchdog reach on the hub, grouped and each with why
- [x] 3.2 Add the hub accessors `Deps` needs that are not yet exported (`Store`, `Sit`,
      `Lifetime`, `Observed`, `Container`, `RepoDocState`)
- [x] 3.3 Move `ticks.go` → `sweep/sweep.go`, converting the table and its lifecycle to functions
      over `Deps`
- [x] 3.4 Move `tick_credentials.go`, `tick_reference.go`, `tick_stalled.go` → `sweep/{credentials,
      reference,stalled}.go`
- [x] 3.5 Move `watchdog.go` → `sweep/watchdog.go` and `watchdog_status.go` → `sweep/status.go`
- [x] 3.6 Have the hub construct the sweeps in `open`, unchanged in order, and expose what the
      board reads off the watchdog
- [x] 3.7 Move each file's tests with it, named after the file they exercise
- [x] 3.8 Update the moved files' `// package:` headers
- [x] 3.9 Green: `go build ./...`, the full suite, all 10 linters

## 4. Close out

- [x] 4.1 Read `brokkr map internal/hub --depth 1` end to end: every header names its new home and
      the root reads as groups
- [x] 4.2 Record in the design what the hub root is left holding, and why each remaining package
      has no parent that would be true for all its callers
- [x] 4.3 Green: `go build ./...`, the full suite, all 10 linters
