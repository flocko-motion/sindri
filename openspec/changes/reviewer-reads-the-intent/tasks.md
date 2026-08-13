# Tasks

## 1. Let it read

- [x] 1.1 Add `reviewer` to the `task` verb's roles.
- [x] 1.2 Give it whole-backlog scope in `visibleTasks`, beside planner and coauthor — a worker's
      bounded scope would show it nothing, since it holds no task.
- [x] 1.3 Print type and labels in `task <id>`. Without this a `spec:` label is undiscoverable, and
      access alone would not have satisfied the spec-driven requirement.

## 2. Tell it

- [x] 2.1 `DirReview` names the task by id and title, as `DirClaimed` does for a worker.
- [x] 2.2 It points at `task <id>`, `task list`, the comments, and the `spec:` label rule.
- [x] 2.3 `DefaultReviewPrompt` says the same, so it holds however the review was requested.

## 3. Show it

- [x] 3.1 The agent-facing `show <pr-id>` names the linked task, as the host PR detail does.

## 4. Grant nothing else

- [x] 4.1 Assert the registry's role lists: the reviewer gains `task` and none of create/edit/
      prioritise/reopen/next/submit. Authority lives there, not in the handlers.
- [x] 4.2 And that the roles which already read still do — the grant is additive.

## 5. Pin it

- [x] 5.1 The task, its hierarchy, its comments and its spec label are all readable.
- [x] 5.2 Both routes into a review mention the verbs; the directive survives an unreadable title.
- [x] 5.3 Mutation-checked compilably: removing the role, narrowing the scope, dropping the intent
      clause, and dropping the labels each fail their own tests.
