# Tasks

This change is documentation-only: it reconciles the specs with the
already-shipped planner role. The "implementation" tasks below are the spec
deltas being authored; no source code changes.

## 1. Author the delta specs

- [x] 1.1 `hub`: state-filtered surface across three roles + the user-approval
      gate on planner-proposed tasks
- [x] 1.2 `agent-runtime`: the planner is a third role that plans and is never
      auto-assigned work (orient-and-wait directive)
- [x] 1.3 `04-workers`: planner mount topology (read-only repo + writable
      openspec overlay), distinct from the identical worker/reviewer mounts
- [x] 1.4 `05-workflow`: plan/build/review separation names the planner agent;
      task proposals are user-gated
- [x] 1.5 `03-gh-local`: planner command surface + planner ships openspec as a PR
      on a standing branch (`plan-<name>`, mock task `os-new`), rebased on merge
- [x] 1.6 `view-tui`: Tasks tab marks gated planner proposals and approves/rejects
      them; new-agent picker offers the planner role
- [x] 1.7 `view-workers`: agents rendered with their own role (worker/reviewer/
      planner); a planner is never shown "working"

## 2. Correct adjacent cross-role drift

- [x] 2.1 `agent-runtime`: the hub names an agent its own role; roster/other
      agents/`.sindri` stay hidden (fix "role invisible to the agent")
- [x] 2.2 `04-workers`: the dwarf-name pool is role-agnostic (fix "reviewer takes
      no dwarf name")

## 3. Verify

- [x] 3.1 `openspec validate --all` passes (run by the submit lint gate)
