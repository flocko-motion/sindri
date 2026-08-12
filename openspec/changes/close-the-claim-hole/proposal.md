# Every open task must be claimable by someone

## Why

A task's status says open or it does not — there is no third state, and nothing may
quietly treat one as unavailable while the board still reads open. Several tasks in
sindri's own backlog were open, rated, ungated, and unheld, yet no claim path ever
served them: two P1 packages sat still while workers took P2 leaves, because nothing
preferred mid over high — the highs were in neither pool.

The hole: `OpenLeaves` excludes any task that ever had a child, forever, regardless of
that child's status. `OpenContainers` wants a DIRECT child that is currently open. A
package whose only child has already closed — because its subtasks were checkpointed
and it just never went up as its own PR — therefore falls between the two, permanently.
Nothing else ever touches its status, so it stays open with no live route to close it.

The obvious wrong fix is dropping the status filter from `OpenLeaves`' anti-parent
check: that would send such a task through `claimLeaf`, which runs
`git.CheckoutDetachedClean` and lays a fresh branch off base — abandoning the branch its
subtasks were checkpointed onto, with their work still on it. The claim has to reuse
that branch, which is `OpenContainers`' path (`git.EnsureBranch`).

The right fix: `OpenContainers` keys on being open with no live route to close it —
"has had a child, and has not landed" — rather than on having an open child right now.
A package whose remaining descendant is merely gated stays excluded (the gate clearing
releases it later); one with nothing left anywhere under it is offered so someone can
claim it and finish it. `claimContainer` holds that shape too, instead of declining it,
so the agent's next directive tells it to submit and close it.

## What Changes

- `store.OpenContainers`' SQL pre-filter drops to "has a child at all" (any status); the
  Go-side filter now includes a candidate when it has real actionable work
  (`OpenSubtasks`) OR nothing left anywhere under it at all (`HasOpenDescendant`,
  a new query) — excluding only the case where real work exists but is merely gated.
- `workflow.claimContainer` holds a container with nothing left the same way as one
  with real work: `git.EnsureBranch` reuses its existing branch, and the agent is told
  to submit and finish it (`DirContainerDone`) rather than declining silently.
- A new invariant test (`TestEveryOpenRatedUngatedUnheldTaskIsClaimable`) states the
  rule directly — every open, rated, ungated, unheld task with nothing left to wait on
  is offered by `OpenLeaves` or `OpenContainers` — rather than enumerating the shapes
  seen so far, so it catches the next cause too.
- `store/workflow.go` split: the cached task read model and its claim queries move to
  their own file, `store/tasks.go` (the file had grown past the 700-line limit).

## Impact

- **Source of truth:** `internal/hub/store/tasks.go` (`OpenContainers`,
  `HasOpenDescendant`), `internal/hub/workflow/feature.go` (`claimContainer`).
- No change to a task with real, ungated work under it — that path is unchanged. The
  only new claimable shape is a container with nothing left and no route to close it.
