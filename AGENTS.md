# SYSIPHOS — Agent Worker Loop

## Overview

Each agent session runs inside a container with three mounts:

- `/workspace` — **the feature branch** (read-write). This is where all work happens.
- `/repo` — **the main branch** (read-only). Whatever the main worktree has checked out.
- `/todos` — shared todo state, visible to the host (read-write)

From the agent's perspective there are exactly two branches:
- **the feature branch** — `/workspace`, the thing being built
- **the main branch** — `/repo`, the merge target

Branch names don't matter. `GH_LOCAL_BASE` is set by the launcher to whatever
the main worktree currently has checked out. The agent never needs to know or care.

`TD_ROOT` points to `/todos` so all `td` commands operate on the shared store.

---

## Worker Loop

```
1.  td usage --new-session
        Start a fresh td session; prints session summary.

2.  td next
        Get the next open issue-id.
        Nothing returned → no work left, exit cleanly.

3.  td start <issue-id>
        Mark issue in-progress. Work on it in /workspace.

4.  Work
        Implement the issue. Tests pass. Changes committed to the feature branch.

5.  gh pr create --title "<summary>" --body "<details>"
        Captures the diff (feature branch vs main branch),
        writes .git/pr/pr-<issue-id>.json, status=open.

6.  td handoff <issue-id> [--done|--remaining "<note>"|--decision "<note>"]
        Update todo state to signal the host that review is ready.

7.  Wait for approval
        Poll: gh pr view | jq -e '.status == "approved"'
        Not approved after timeout → exit 1.
        State is fully preserved — pod restart picks up where it left off.

8.  gh pr merge
        Merges the feature branch into the main branch.
        Blocked with exit 1 if not approved.

9.  goto 1
```

---

## Approval Gate

`gh pr merge` exits non-zero if `status != approved`.
The agent **cannot** self-merge. Approval comes from the human via the host.

```bash
# host: review and approve
gh pr view              # inspect
gh pr review --approve  # unblock the agent
```

---

## State Persistence

| What             | Where                  | Survives pod restart? |
|------------------|------------------------|-----------------------|
| Todos / issues   | `/todos/` (host vol)   | Yes                   |
| PR metadata      | `.git/pr/` (host vol)  | Yes                   |
| Git commits      | `/workspace` (host vol)| Yes                   |
| Session memory   | container ephemeral    | No                    |

Throw the container away freely. All meaningful state lives on the host.

---

## Volumes (set by launcher)

```
<main-repo>/          → /repo       (ro)  — the main branch
<worktree>/           → /workspace  (rw)  — the feature branch
<main-repo>/.todos/   → /todos      (rw)  — shared todo state
```
