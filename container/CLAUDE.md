# Sindri Worker Agent

You are a Sindri worker agent running inside a sandboxed container.

## Your environment

- `/workspace` — your worktree (read-write). This is a git worktree. Commit here.
- `/repo` — the main repository (read-only source files, read-write `.git`). Do NOT edit files here.
- `/project/.todos` — shared task database.

## Git setup

Your `/workspace/.git` is a worktree pointer to `/repo/.git/worktrees/<your-name>`.
You start on the base branch. `gh issue next` creates a per-task branch automatically.

## Tools

- `gh` — sindri-local workflow engine (NOT GitHub). Manages tasks and PRs locally.
  - `gh issue next` — pick up next task (auto: claim, rebase, branch, show details+comments)
  - `gh issue view` — show current task details + comments (auto-detects from branch)
  - `gh issue comment -b "msg"` — comment on current task
  - `gh issue list` — list open tasks
  - `gh submit --title "..."` — submit work (auto: rebase, PR, handoff, review, wait)
  - `gh done` — return to base branch for next task
  - `gh pr list/view` — inspect PRs
- Standard dev tools: git, python3, pytest, go, node, npm, etc.

## Rules

- Do NOT use `EnterWorktree` or `ExitWorktree` — you are already in a worktree.
- Do NOT edit files in `/repo` — it is read-only (except `.git`).
- Do NOT merge PRs — that is the reviewer's job.
- Do NOT use `td` directly — use `gh issue` commands instead.
- Ask before guessing when requirements are unclear.

## TUI conventions

When working on the TUI (internal/tui/):
- Navigation between columns/panels: ctrl+h (left), ctrl+l (right), ctrl+j (down), ctrl+k (up)
- List navigation within a panel: j/k
- Detail pane scrolling: Shift+J / Shift+K
- No tab key for navigation — use ctrl+h/l instead
