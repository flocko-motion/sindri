# Tasks

## 1. A gate describes a commit

- [x] 1.1 `gateCommit` records the agent's workspace as a commit (the message its own summary would
      have carried) and returns the sha; `git.Head` is the adapter helper, reused by `AttachBranch`.
- [x] 1.2 `CmdSubmit`/`CmdContribute`/`CmdLint` commit BEFORE the gate opens, and park the agent
      before it can settle — a reused pass lands its PR inside the next call.
- [x] 1.3 `landSubmit`/`openMilestoneOrInterim` no longer commit: the commit is what was gated.
- [x] 1.4 The commit happens only in a worktree the hub owns (`hubOwnedTree`): a coauthor's shared
      checkout and a reviewer's detached tree are checked as they stand, and a clean one is still
      named by its HEAD so a reviewer reuses the submit gate's pass.
- [x] 1.5 A gate run row carries the commit it checks (`runs.commit_sha`), and `gateTree` checks that
      commit out fresh (`repo.MaterializeGate`) for every kind that has one — a self-check parks
      nobody, so its live tree moves under the run.

## 2. The verdict is stored against the commit

- [x] 2.1 `gate_result` (project, sha, passed, verify, output, ran_at) with `SetGateResult` and
      `GatePassed`; `pr_lint` gains the sha the result describes.
- [x] 2.2 `gateRun` settles a commit with a stored PASS at once, without queueing; `CmdLint` and
      `LintPR` answer from the store the same way.
- [x] 2.3 Every reused answer says so and names the commit (`gateReusedReport`).
- [x] 2.4 A stored pass under a different `verify` is not reused; a stored FAILURE decides nothing
      (`GatePassed`) but reads back (`GateVerdict`), so the message pointing a reviewer at a failed
      verdict does not queue a fresh gate every time it looks.
- [x] 2.5 A reused verdict's row is written already settled, so the run watcher cannot execute for
      real what the reuse path is finishing.

## 3. One linter per gate

- [x] 3.1 `repo.Gate` runs the declared verify OR the built-in lint, never both; `builtinLint` loses
      the `declared` flag it only used to document the non-Go case.

## 4. Every gate goes through the queue

- [x] 4.1 `sindri lint` becomes `workflow.CmdLint` (kind `lint`), queued, its result injected; the
      hub's inline `cmdLint` is gone, and with it `repo.Gate` from `internal/hub`.
- [x] 4.2 `LintPR` (kind `lint-pr`) gates the branch's commit in `.worktrees/gate`; a second ask
      joins the queued run rather than queueing another, and the wait is stored on the PR.
- [x] 4.3 The open-PR precheck (kind `precheck`) queues its materialise-and-gate; `CheckOpenPRs`
      only decides, and marks the PR answered when the run is QUEUED so a 30s sweep cannot pile up.
- [x] 4.4 A precheck ranks as an ordinary run (`gateBlocksSomeone`); a gate on a PR carries no agent
      that can go stale (`gateOnAPR`).
- [x] 4.5 The TUI's lint pane reports queued and running, not just PASS/FAIL, says "asking" rather
      than "running" while the hub answers, and labels the result with the commit it describes
      (`PRDetail.LintCommit`, read rather than only served).
- [x] 4.6 Everyone who asked is told: `run_waiters` records each agent asker, and `completeLintPR`
      messages the list rather than the run's own asker — a joining agent would otherwise wait on a
      message nobody sends, and the human keeps the queue lead their run was given.
- [x] 4.7 The CLI's `pr lint` help and the client's doc say what it now does (a verdict about a
      commit, possibly answered without running anything), matching the care the TUI got.

## 5. `git restore` becomes `git rollback`

- [x] 5.1 `git rollback <id>` resets the workspace to a point in the agent's own history; `restore`
      is refused with what to use instead, and `git.RestoreFromHEAD` is gone with it.
- [x] 5.2 The id must resolve, be in this branch's history, and be no earlier than where the branch
      left the reference — each refusal naming what to do instead, and moving nothing.
- [x] 5.3 The pod git shim and the agent briefs name the new verb.
- [x] 5.4 Neither `rollback` nor `drop` runs against a shared checkout — a coauthor's `/workspace` is
      the user's own tree, and `git drop` had the same hazard before this.

## 6. Traps closed while here

- [x] 6.1 A landing verb whose gate fails to open puts the phase back: `gating` has no way out of its
      own, so the agent would be parked with nothing coming.
- [x] 6.2 `refwatch.preflight`'s comment no longer justifies itself by a gate that may run for
      minutes — it only decides and queues now.

## 7. check-go leaves the gate

- [x] 7.1 `make verify` drops the `check-go` prerequisite; CI runs it as its own step and
      `make install` still depends on it.

## 8. Pin it

- [x] 8.1 A commit with a stored pass opens its PR without a gate running; a changed tree is gated
      again.
- [x] 8.2 A self-check leaves the agent's phase alone, pass or fail — only a landing verb parks one.
- [x] 8.3 A PR check reuses the submit gate's pass, and gates the branch rather than the author's
      tree (whose uncommitted work is left untouched).
- [x] 8.4 A declared gate's presence keeps the built-in linter from running.
- [x] 8.5 A self-check measures the commit it was queued for even when the agent edits its workspace
      while it waits, and a recorded failure reads back without queueing anything.
- [x] 8.6 A rollback discards everything after its target, including work never handed over, and
      refuses an id that is not the agent's own.
- [x] 8.7 `go test ./...` and `brokkr lint` pass.
