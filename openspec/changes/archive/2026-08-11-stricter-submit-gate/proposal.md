# A stricter submit gate: the project supplies its own

## Why

The gate that stands between an agent's work and a PR is narrower than it looks.
`internal/hub/repo/repo.go:44-56` is the whole of it:

```go
if _, err := os.Stat(filepath.Join(wt, "go.mod")); err != nil {
	return "", true // no Go module — no lint gate applies
}
...
cmd := exec.Command(bin, "lint")
```

So the gate is `brokkr lint` and nothing else. It does not build, does not run tests,
and returns a clean pass for any project without a `go.mod` — a repository in another
language has no gate at all. Meanwhile this project's real gate is `make verify`
(build, `go test ./...`, `brokkr lint`), which CI runs on every PR and an agent never
runs before submitting.

That is the wrong way round. The checks a project cares about are the project's to
declare, and they are exactly the ones an agent should not be able to submit past. And
they cannot go into `brokkr`: it is a generic toolbelt, deliberately kept out of the
product, so a rule about *this* repository's architecture has no place in it.

The consequence today is that every project-specific invariant — the front-end import
boundary, the layer vocabulary, anything a test can assert — is caught by CI or by a
human reviewer after the PR exists, rather than refused at submit.

## What Changes

- **A `verify:` key in `.sindri/config.yaml`** names a repo-relative executable the
  submit gate runs in the worktree, alongside the built-in gates. Repo-relative and
  validated like every other path key: absolute, escaping, or missing is invalid
  config, loudly.
- **The gate runs it after the rebase and before the PR record exists**, the same slot
  the lint gate occupies. A non-zero exit refuses the submit and reports the output, so
  a failing PR is never created.
- **A configured gate runs whatever the language.** The `go.mod` early return stops
  being a silent pass: when a project declares a verify command, it runs; when it
  declares none, the built-in Go gates apply as they do today.
- **The output is kept and shown.** It is stored on the PR like the lint output already
  is (`PRDetail` carries `Lint` and `LintAt`) and surfaced by `sindri pr lint`, so a
  human sees why a submit was refused without re-running anything.
- **It is bounded.** A timeout, and output capped the way the rest of the system caps
  it — saying what was cut and how much, because silent truncation reads as a complete
  answer.

## Capabilities

### Modified Capabilities

- `03-gh-local`: the submit gate is the built-in checks plus the project's own declared
  verify command; it applies regardless of the project's language once declared, and
  its output is retained on the PR.

### Added Capabilities

- `project-config`: a `verify` key naming the project's own gate command.

## Impact

- **Source of truth:** `internal/config` (the new key + validation),
  `internal/hub/repo/repo.go` (`Lint` becomes the gate runner: built-ins plus the
  configured command, timeout, capped output), the PR record's stored gate output, and
  `sindri pr lint`'s rendering of it.
- **This repository** then sets `verify: scripts/verify.sh` wrapping `make verify`, so
  an agent cannot submit a PR that fails the build, the tests, or the architecture
  tests.
- Existing repositories are unaffected: with no `verify` key the gate behaves exactly
  as it does today.
