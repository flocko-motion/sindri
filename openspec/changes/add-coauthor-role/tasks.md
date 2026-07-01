# Tasks

This change is documentation-only: it reconciles the specs with the
already-shipped coauthor role. The "implementation" tasks below are the spec
deltas being authored; no source code changes.

## 1. Author the delta specs

- [x] 1.1 `agent-runtime`: the coauthor is a fourth role driven directly (never
      auto-assigned, never blocking); agent isolation's one exception — a coauthor
      shares the user's checkout, with `.sindri/` still hidden
- [x] 1.2 `04-workers`: coauthor mount topology (user's repo root read-write,
      `.sindri/` shielded), role-agnostic naming across four roles, and the user's
      skills mounted into every Claude agent
- [x] 1.3 `hub`: state-filtered surface across four roles; the coauthor's
      helper-only surface
- [x] 1.4 `05-workflow`: the coauthor works outside the managed loop — no task, no
      managed PR, no review gate; it uses git itself
- [x] 1.5 `03-gh-local`: role-scoped commands include the coauthor's helper-only
      surface
- [x] 1.6 `view-tui`: the new-agent picker offers the coauthor role
- [x] 1.7 `view-workers`: agents rendered with their own role (incl. coauthor); a
      coauthor's status is down/idle/collab, never working

## 2. Verify

- [x] 2.1 `openspec validate --all` passes (run by the lint gate)
