# Sindri

A sandboxed AI agent that works through issues, submits PRs, and waits for human approval — in a loop.

---

## Concept

Sindri runs an AI agent inside a container, pointed at a git worktree. The agent picks up issues, writes code, creates PRs, and blocks until a human approves. Then it merges and moves to the next issue.

The agent speaks `gh` — the same CLI interface whether running fully local or against GitHub. A pluggable backend handles the routing.

```
┌───────────────────────────────────────────────┐
│  Host                                         │
│                                               │
│  Human reviews via:                           │
│    gh issue list / comment                    │
│    gh pr view / review --approve              │
│    td monitor  (td backend only)              │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │  Container (Podman)                      │ │
│  │                                          │ │
│  │  Agent (Claude Code)                     │ │
│  │    gh issue list / view / comment        │ │
│  │    gh pr create / merge                  │ │
│  │                                          │ │
│  │  gh binary routes to backend:            │ │
│  │    td   → fully local, offline           │ │
│  │    gh   → GitHub issues + PRs            │ │
│  │                                          │ │
│  │  Mounts:                                 │ │
│  │    /repo      ← main repo (read-only)   │ │
│  │    /workspace ← worktree (read-write)    │ │
│  └──────────────────────────────────────────┘ │
└───────────────────────────────────────────────┘
```

---

## The loop

```
1.  gh issue list --state open         ← pick an issue
2.  work on it                         ← code, test, commit
3.  gh pr create --title "..."         ← submit for review
4.  wait for approval                  ← poll gh pr view
5.  gh pr merge                        ← blocked until approved
6.  goto 1
```

The agent **cannot merge without human approval**. That's the only hard gate.

---

## Communication

Agent and human talk through issue comments.

```bash
# Agent is stuck — asks a question
gh issue comment 42 -b "Should the cache TTL be configurable or hardcoded?"

# Human replies
gh issue comment 42 -b "Configurable, default 300s"

# Agent reads comments, continues
gh issue view 42 --comments
```

This works identically in both backends.

---

## Backends

### td (local)

Issues, comments, and state live in a local td database. No network required.

The agent's `gh` binary translates commands to `td`:

| Agent runs | Backend executes |
|---|---|
| `gh issue list` | `td list` |
| `gh issue create` | `td create` |
| `gh issue comment` | `td comment` |
| `gh pr create` | local PR store (`.git/pr/`) |
| `gh pr review --approve` | local PR store |
| `gh pr merge` | `git merge` (if approved) |

Human gets `td monitor` for a live TUI dashboard, plus all `td` query and filtering features.

### GitHub

Issues and PRs live on GitHub. The agent's `gh` binary is the real GitHub CLI. Standard remote workflow — nothing special.

---

## Quick start

```bash
# Create a worktree for the agent
git worktree add ../my-feature my-feature

# Start a worker with an instruction
sindri work ../my-feature -p "Implement the login page per issue #12"

# Or start interactive (no predefined task)
sindri work ../my-feature
```

On first run the container image is built automatically.

---

## Approval workflow

```bash
# See what the agent submitted
gh pr list
gh pr view <id>

# Approve — this unblocks the agent
gh pr review <id> --approve

# Or with td backend, use the monitor
td monitor
```

---

## Environment

| Variable | Default | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | — | Required for Claude Code |
| `GH_LOCAL_BASE` | auto-detected | Target branch for merges |
| `SINDRI_BACKEND` | `td` | Backend: `td` or `github` |

---

## Repo layout

```
sindri              ← entry point
AGENTS.md           ← agent loop documentation
gh-local/           ← gh CLI adapter (Go) — routes to td or github
shims/              ← docker → podman shims
dockerfiles/        ← container image
agent/              ← pod definitions
```

---

## State persistence

Everything meaningful lives on the host, not in the container.

| What | Where |
|---|---|
| Code / commits | worktree (host) |
| Issues / comments | td database or GitHub |
| PR metadata | `.git/pr/` (td mode) or GitHub |

Throw the container away freely. Restart picks up where it left off.

---

## Acknowledgments

The Sindri TUI is built on [sidecar](https://github.com/marcus/sidecar) by Marcus — a terminal dashboard for AI coding agents. We forked the Bubble Tea plugin architecture, td monitor integration, and UI primitives as the foundation for Sindri's orchestration interface.
