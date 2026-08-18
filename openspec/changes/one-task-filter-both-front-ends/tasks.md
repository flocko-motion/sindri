# Tasks

## 1. Move the rule to the exchange package

- [x] 1.1 The four filter values, the predicate, and the window as one constant in `internal/api`.
- [x] 1.2 "Active" written down as the union it is: not done, plus changed inside the window.
- [x] 1.3 The order both front-ends present, so the CLI's help and the TUI's cycle agree.
- [x] 1.4 `api.DoneStatus` names the terminal statuses once; the TUI's `isDone` copy goes, row
      colouring included.

## 2. Two callers, one definition

- [x] 2.1 The TUI's `taskRows` filters through `api.FilterTasks`, and `f` cycles `api.TaskFilters`.
- [x] 2.2 `sindri task list --filter`, defaulting to `all` so no existing use changes.
- [x] 2.3 The listing says when a filter hid tasks, and the verdict count still counts every task.

## 3. Date what was undated

- [x] 3.1 The openspec source dates each change from the newest mtime under its directory.
- [x] 3.2 The path guard the other change-name callers use is one function, applied here too.

## 4. Pin it

- [x] 4.1 Each filter against one backlog, including the active union and a merged task.
- [x] 4.2 An undated task: never recent, still reachable under closed and all.
- [x] 4.3 A bad `--filter` value is rejected and names the set; an unknown filter admits everything.
- [x] 4.4 The cycle visits each filter once and wraps.
- [x] 4.5 A change is dated from its newest file, and a name that would escape the directory is not.
