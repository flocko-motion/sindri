---
name: td-choose
description: Discuss open tasks with the user and help them decide what to work on next.
---

Run `td -w /project ready` to list all open tasks and `td -w /project status` for the current state.
Present a summary and discuss with me what to work on next.
Ask questions, suggest priorities, help me decide. Wait for my decision.

Once I decide:
1. `td -w /project start <id>` to claim it
2. `td -w /project show <id>` to read the details
3. If anything is unclear, ask me before guessing
4. Implement the task, run tests, commit your changes
5. `gh pr create --title "summary" --body "details"` to submit a PR
6. `td -w /project handoff <id> --done "what you did"`
7. `td -w /project review <id>` to submit for review

The main repo is at /repo (read-only). Your workspace is /workspace.
All td commands need `-w /project`.
