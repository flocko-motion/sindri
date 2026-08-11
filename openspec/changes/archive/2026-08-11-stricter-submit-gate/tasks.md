# Tasks

## 1. The config key

- [ ] 1.1 Add `Verify string` to the project config with a `verify` yaml tag, and
      validate it as a repo-relative path that must exist when set — the same
      fail-loud path validation `architecture`, `containerfile` and `review_prompt`
      already use.
- [ ] 1.2 Cover it in the config tests alongside the existing path-key cases
      (`TestLoadValid`, `TestLoadRejects`).

## 2. The gate runs it

- [ ] 2.1 Turn `internal/hub/repo/repo.go`'s `Lint` into the gate runner: the built-in
      checks plus the configured command, run in the worktree after the rebase and
      before the PR record exists.
- [ ] 2.2 Remove the silent pass for a project with no `go.mod` when a verify command
      is declared: the declared gate decides, whatever the language. With no command
      declared, keep today's behaviour.
- [ ] 2.3 Bound it: a timeout that reports as a refusal rather than blocking the agent,
      and output capped the way `hub/workflow/gitcmd.go` caps its diffs — naming what
      was cut and how much.
- [ ] 2.4 A non-zero exit refuses the submit and reports the output to the agent, so no
      PR record is created.

## 3. The output is kept and shown

- [ ] 3.1 Store the gate output on the PR record, reusing the fields the lint output
      already occupies (`Lint`, `LintAt` in the PR detail).
- [ ] 3.2 Show it in `sindri pr lint` and the PRs tab detail, so a human sees why a
      submit was refused without re-running the gate.

## 4. This repository uses it

- [ ] 4.1 Add `scripts/verify.sh` wrapping `make verify` (build, `go test ./...`,
      `brokkr lint`), and set `verify: scripts/verify.sh` in `.sindri/config.yaml`.
- [ ] 4.2 Confirm an agent can no longer submit a PR that fails the build, the tests,
      or the architecture tests.

## 5. Verify

- [ ] 5.1 `make verify` passes.
- [ ] 5.2 `openspec validate --all` passes.
- [ ] 5.3 Manual: a worker submitting deliberately failing work is refused with the
      command's output, and the same PR shows that output afterwards; a repo with no
      `verify` key still submits exactly as before.
