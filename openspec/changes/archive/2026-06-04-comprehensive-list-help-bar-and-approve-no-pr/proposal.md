# List-view help bar lists every binding; approve closes a no-PR task

## Why

Two follow-ups on the td-e035de review comments that I didn't fully
address last time:

1. The list-view help bar only listed `n:new e:edit r:refresh q:quit`
   — bindings `a` (approve), `x` (reject/abandon), `s` (status),
   `m` (move), `c` (comment), `f` (filter) accepted the same keys but
   weren't advertised. The user: "there's still no `a` approve hotkey
   in list view — at least not visible". The td-8c5ba9 rule ("help
   bar lists every list-view binding") was being violated on every
   binding except edit.

2. Pressing `a` on a task without a PR surfaced
   "Approve: this task has no PR yet" and did nothing. The user:
   "approve/reject should also work when there's no PR, because we
   don't always work with PRs. If there's no pr, then we just close
   the item by approving."

## What Changes

**Help bar redesign.** Moved out of the right side of the title row
into its own dedicated row beneath the title. Grouped into three
` · `-separated chunks (navigation, row actions, view actions) so the
eye can split them quickly:

```
j/k:nav enter:open · y:copy n:new e:edit a:approve x:reject s:status m:move c:comment · f:filter r:refresh q:quit
```

The workers view uses the same row but drops the row actions (they
only apply to tasks):

```
j/k:nav enter:open · r:refresh q:quit
```

**Approve without PR closes the task.** New `action.ApproveTask`
closes the linked td task with reason `"approved"`. Both the list-view
`a` handler and the detail-view `a` handler now route through it when
there's no PR — the previous error notification path is gone. The
close goes through the same status-changed pipeline as the status
picker, so the linked-spec lifecycle check fires for free (auto-archive
if this was the last linked task with a complete checklist; prompt if
incomplete).

## Impact

- `view-work-list` MODIFIED — help bar requirement now scopes the
  exact contents and pins the dedicated row.
- `action-approve` MODIFIED — approve now has two paths: with PR
  (existing) and without PR (close).
- Code touched: `internal/tui/backlog.go` (help-bar row, grouped
  layout, view-conditional contents), `internal/tui/list_view_test.go`
  (overflow check skips the help-bar row, since it's fixed chrome),
  `internal/tui/tui.go` (list-view `a` handler routes through the
  no-PR path), `internal/tui/update_detail.go` (same routing for the
  detail view), `internal/tui/actions.go` (`approveTaskNoPR` Tea
  command), `internal/action/action.go` (`ApproveTask` task-closing
  shared action).
- Goldens regenerated. Layout shifts down by one row everywhere.
