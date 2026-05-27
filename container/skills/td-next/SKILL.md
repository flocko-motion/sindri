---
name: td-next
description: Pick up the next task from td and work on it autonomously.
---

Run `td -w /project next` to find the highest-priority open task.
If no task is available, say so and wait for instructions.

When you have a task:
1. `td -w /project start <id>` to claim it
2. `td -w /project show <id>` to read the details
3. `td -w /project comments <id>` to read reviewer feedback (IMPORTANT — may contain rejection reasons or clarifications)
4. Run `/statusline <task-id>: <title>` to show the current task in the statusline
5. If anything is unclear, ask me before guessing
6. Implement the task, run tests, commit your changes
7. `gh pr create --task <id> --title "type(task-id): summary" --body "details"` — use conventional commits:
   - feature → `feat(td-xxx): ...`
   - bug → `fix(td-xxx): ...`
   - task/chore → `chore(td-xxx): ...`
   The td issue type tells you which prefix to use.
8. `td -w /project handoff <id> --done "what you did"`
9. `td -w /project review <id>` to submit for review
10. The gh tool will wait for review approval automatically.
11. Once approved, go back to step 1 — pick up the next task.

The `gh` command manages local PRs (no GitHub needed).
The main repo is at /repo (read-only). Your workspace is /workspace.
All td commands need `-w /project`.
Do NOT use EnterWorktree. Work directly in /workspace.
