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

## 2b. Make the instruction actually reach a project

- [x] 2.4 Stop writing the default into the project's state on first use — that copy is what froze
      it, and nothing reported the divergence.
- [x] 2.5 Treat a stored instruction matching one sindri wrote (current or superseded) as the seed,
      not a choice, so the improvement reaches installs that already hold the file. Stopping the
      write alone would have fixed only projects that never had a review.
- [x] 2.6 An edited instruction still wins.

## 3. Show it

- [x] 3.1 The agent-facing `show <pr-id>` names the linked task, as the host PR detail does.
- [x] 3.2 Through a cache read, not `TaskInfo`: a display verb must not write.

## 4. Grant nothing else

- [x] 4.1 Assert the registry's role lists: the reviewer gains `task` and none of create/edit/
      prioritise/reopen/next/submit. Authority lives there, not in the handlers.
- [x] 4.2 And that the roles which already read still do — the grant is additive.

## 5. Pin it

- [x] 5.1 The task, its hierarchy, its comments and its spec label are all readable.
- [x] 5.2 Both routes into a review mention the verbs; the directive survives an unreadable title.
- [x] 5.3 The prompt from every side: not written out, an improved default reaching a project that
      holds the old seed, an edited one still winning, and today's default counting as a seed too so
      the next improvement is not stuck again.
- [x] 5.4 Mutation-checked compilably: removing the role, narrowing the scope, dropping the intent
      clause, dropping the labels, letting a stored file always win, seeding the default back to
      disk, and resolving the task through `TaskInfo` each fail their own tests. Two of those
      mutations were ineffective on the first attempt (one broke the build, one wrote to a missing
      directory and failed silently) and proved nothing until redone.
