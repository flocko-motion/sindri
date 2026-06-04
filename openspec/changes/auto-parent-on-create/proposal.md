# Auto-parent on create when cursor is on an epic or feature

## Why

Today every new task lands at the root of the work list. When the user
is navigating an epic and presses `n` to add a sub-item to it, they
have to immediately switch to move mode (`m h/l`) to re-parent the new
task under the epic. The natural assumption is the new item belongs
under the row they were looking at — exactly the user's framing:

> when i press n to creat a new item and the new item is not a feature
> or epic then create it as a child of this feature.... or better: i
> epic selected, make all non-epics a child.. if feature selected,
> make all non-epic non-feature a child

The auto-parent rule reads off the umbrella the cursor sits on:
items "smaller" than the cursor row land underneath it; items at the
same or larger scope stay at the root.

## What Changes

- `td.CreateOpts` gains a `Parent` field; `td.Create` appends
  `--parent <id>` when set.
- `createTaskModel` captures the cursor row's `(taskID, taskType)` at
  modal-open time as `cursorParentID` / `cursorParentType`.
- New pure helper `resolveAutoParent(parentID, parentType, newType)`
  encodes the rule:
  - cursor on `epic` → any non-epic new task gets it as parent.
  - cursor on `feature` → any new task that is neither an epic nor a
    feature gets it as parent.
  - cursor on anything else → no auto-parent.
- The modal renders an "Auto-parent: td-XXX (cursor on epic|feature)"
  line whenever the rule applies for the *currently selected* type,
  so flipping the type selector immediately shows/hides the effect.
  The user sees the parent before submitting and can switch the type
  to opt out.
- Submit time: `submit()` calls `resolveAutoParent` and passes the
  resulting `Parent` to `td.Create`.

The rule is create-only. The edit modal (`e`) never re-parents.

## Impact

- New requirement on `action-create-task` covers the rule + the
  `td.CreateOpts.Parent` contract.
- New scenario on `view-work-list` ties the rule to the `n` hotkey.
- Affected code: `internal/adapter/td/td.go`,
  `internal/tui/create_task.go`, `internal/tui/actions.go` (cursor
  helper), `internal/tui/tui.go` (n handler wiring).
- `TestResolveAutoParent` pins all 14 branches of the rule.
- New goldens: `create-auto-parent-epic`,
  `create-auto-parent-feature`, `create-no-auto-parent-bug`. Existing
  goldens unchanged (rule fires only with an epic/feature under the
  cursor, which `SimpleFixture` doesn't have).
