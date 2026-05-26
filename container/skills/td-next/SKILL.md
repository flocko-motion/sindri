---
name: td-next
description: Pick up the next task from td and work on it autonomously.
---

Run `td -w /project next` to find the highest-priority open task.
If no task is available, say so and wait for instructions.

When you have a task:
1. `td -w /project start <id>` to claim it
2. `td -w /project show <id>` to read the details
3. If anything is unclear, ask me before guessing
4. Implement the task, run tests, commit your changes
5. `gh pr create --title "summary" --body "details"` to create a local PR
6. `td -w /project handoff <id> --done "what you did"`
7. `td -w /project review <id>` to submit for review
8. STOP. Your job is done. Do NOT merge — that is the reviewer's job.

The `gh` command manages local PRs (no GitHub needed).
The main repo is at /repo (read-only). Your workspace is /workspace.
All td commands need `-w /project`.
Do NOT use EnterWorktree. Work directly in /workspace.
