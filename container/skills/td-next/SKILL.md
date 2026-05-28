---
name: td-next
description: Pick up the next task from the queue and work on it autonomously.
---

Run `gh issue next` to pick up the highest-priority open task.
This automatically: claims the task, rebases, creates a per-task branch,
and prints the full details with comments.

If no task is available, say so and wait for instructions.

The statusline updates automatically — `gh issue next` writes the task there.
Do NOT try to invoke any statusline skill or command.

When you have a task:
1. If the task has a `spec:<name>` label, it implements an openspec change.
   Run `openspec show <name>` to read the spec FIRST — its requirements and
   scenarios define what "done" means. Implement to satisfy the spec exactly.
2. If anything is unclear, ask me — or comment with `gh issue comment -b "question"`
3. Implement the task, run tests, commit your changes
4. `gh submit --title "type(task-id): summary"` — use conventional commits:
   - feature → `feat(td-xxx): ...`
   - bug → `fix(td-xxx): ...`
   - task/chore → `chore(td-xxx): ...`
   The task type tells you which prefix to use.
5. `gh submit` handles everything: rebase, PR creation, handoff, review submission.
6. When done, run `gh done` then go back to `gh issue next` for the next task.
   Do NOT wait for review — move on to the next task.
7. If a previous task was rejected, `gh issue next` will surface it again with
   the reviewer's comments — read them and fix the issues.

The `gh` command is sindri-local (NOT GitHub). All operations are local.
Do NOT use `td` directly — use `gh issue` commands instead.
Do NOT use EnterWorktree. Work directly in /workspace.
