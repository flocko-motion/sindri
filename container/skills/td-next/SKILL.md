---
name: td-next
description: Pick up the next task from the queue and work on it autonomously.
---

Run `gh issue next` to pick up the highest-priority open task.
This automatically: claims the task, rebases, creates a per-task branch,
and prints the full details with comments.

If no task is available, say so and wait for instructions.

When you have a task:
1. Run `/statusline <task-id>: <title>` to show the current task in the statusline
2. If anything is unclear, ask me — or comment with `gh issue comment -b "question"`
3. Implement the task, run tests, commit your changes
4. `gh submit --title "type(task-id): summary"` — use conventional commits:
   - feature → `feat(td-xxx): ...`
   - bug → `fix(td-xxx): ...`
   - task/chore → `chore(td-xxx): ...`
   The task type tells you which prefix to use.
5. `gh submit` handles everything: rebase, PR creation, handoff, review submission.
   It will wait for approval automatically.
6. If rejected, read the comments with `gh issue view` and fix the issues, then `gh submit` again.
7. When approved, run `gh done` then go back to `gh issue next`.

The `gh` command is sindri-local (NOT GitHub). All operations are local.
Do NOT use `td` directly — use `gh issue` commands instead.
Do NOT use EnterWorktree. Work directly in /workspace.
