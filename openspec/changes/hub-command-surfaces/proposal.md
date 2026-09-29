## Why

The hub serves two command surfaces — the agent's (32 verbs through one `POST /exec`) and
the front-end's (63 REST routes) — and neither has a home. The agent's list, its mechanism,
its transport and six of its implementations are spread across `hub/commands.go`,
`hub/registry/`, `hub/flow/agent/verb/`, `hub/flow/agent/verbs/` and `hub/agent/agentchan/`;
the front-end's 63 routes sit in four files of package `hub` beside the watchdog and the
wiring. A verb is defined twice (`verb.Git.Help` and `verbs.GitHelp` both reach the agent,
and nothing holds them in step), and where two surfaces drive one operation the repo answers
three different ways: `reject` and `resume` share a core, `approve` and `run` are forked, and
`log` names a write on one side and a read on the other.

## What Changes

- `internal/hub/api/` groups the two surfaces, as `internal/hub/messaging/` groups its two
  channels: `api/agents/` for the agent surface, `api/frontend/` for the front-end's routes,
  `api/serve/` for the request conventions both share. It declares no package of its own, so
  it never collides with `internal/api`.
- The verb catalogue collapses to one data-only package, `api/agents/verb`: 32 `Def` values
  carrying name, summary, usage, role and tailored help. `Def` holds no `Run` field, so the
  flow tree's declarations still provably cannot name a writer
  (-> `internal/arch/flow_test.go`).
- `hub/commands.go` shrinks to the binding table — verb name to implementation — and the six
  verbs it still implements (status, log, comment, staff, meeting, reopen-task) move to the
  subject that owns each.
- `contribute` moves to `flow/pr`, whose acting half it already calls three times;
  `flow/agent/verbs/` becomes `flow/agent/workspace/`, holding `git` and `scratch`.
- Every operation both surfaces drive gets ONE exported core in its subject package, with a
  verb adapter and a route adapter over it. `approve` and `run` join `reject` and `resume` in
  that shape.
- `GET /log` becomes `GET /activity`, so one name stops meaning a write to an agent and a
  read to a front-end. The agent's `log` verb keeps its name.
- Two guards enforce the result: an `internal/arch` test that every `CmdX` and every route
  handler bottoms out in a shared exported core, and a `brokkr lint` rule pinning each verb's
  registered help against its `Def`.
- `internal/hub/commands/` (dashboard tabs) becomes `internal/hub/sections/`, and
  `internal/hub/sections.go`'s lone `PROpen` moves in with it.

No agent-visible behaviour changes: the verb names, their help and their gating are carried
through unaltered.

## Capabilities

### New Capabilities

- `command-surfaces`: the two command surfaces the hub serves, what separates them, where a
  verb is defined, and the rule that one operation has one core however it is reached.

### Modified Capabilities

<!-- None: the surfaces keep the behaviour they have, and the requirements describing it
     (hub's "Command surface is state-filtered" and "Protocol is HTTP/JSON carrying repo
     context") stay true as written. -->

## Impact

- **Moved, no behaviour change**: `hub/registry/` → `api/agents/registry/`;
  `hub/agent/agentchan/` → `api/agents/channel/`; `hub/server/` → `api/serve/`;
  `hub/server{,_mail,_runs,_streams}.go` → `api/frontend/`; `flow/agent/verb/` →
  `api/agents/verb/`. `registry.Caller` is the agent-verb calling convention, so 24 non-test
  files take a new import path.
- **One line of arch**: `flowMayImport` swaps `hub/flow/agent/verb` for `hub/api/agents/verb`
  (`internal/arch/flow_test.go:38`).
- **Behaviour converges**: `ApprovePR` and `CmdApprove` (`flow/pr/verdict_act.go:25,120`) are
  two implementations of one operation today; on a shared core the user's approve gains the
  activity-log line and the ordering the agent's already has.
- **Out of scope**: a user verdict leaves an assigned reviewer's row open, on both the
  approve and the reject path. That predates this change and stays as it is.
