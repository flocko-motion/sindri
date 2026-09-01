# One surface for what is allowed

## Why

The rules deciding what may happen to an agent are spread across 23 call sites in 11 files, and each
site re-derives them from raw facts. They drift, and the drift is invisible until an agent behaves
oddly.

The most recent instance is exact: commit `481f6e2` added the `agentBlocked` guard to
`nudgeIdleWorkers` and, in the same change, created `AssignPendingWork` without it — three lines
away. A retired agent is now pushed "sd-… is ready for you" every thirty seconds and told it is
retired when it obeys. The two halves of one rule live in different places and disagree.

Five functions already answer questions of this kind, in three packages, with five signatures:

    api.AgentNeedsUser(a AgentView) bool                             pure, struct in
    workflow.Stalled(phase, container, runtime, waitingOnHub, …)     pure, loose params
    workflow.(*Engine).agentBlocked(ps, project, agent) string       reads the store, returns a reason
    agent.(*Service).HoldsNothing(project, name, role) (bool, error) reads the store
    agent.(*Service).AtLeafBoundary(project, name) (bool, error)     reads the store

Only the first has never drifted. It is also the only one written as a rule over a state struct,
with its own file stating why: "written once so all of them count one set".

**The pattern is already proven here, in a second domain.** `registry.Surface(Caller) []Offered` is
exactly this: a situation struct in, a menu of actions out, each with the reason it is unavailable.
It decides what an agent may TYPE. Nothing decides what the hub may DO to an agent — that is the
scattering.

## What changes

- **One situation struct** — the situation the agent is in — carries its roster row (role, retired,
  clear armed, stopped), its workflow state (phase, task, container, escalation), the observer's
  reading (up, runtime, clients, still-for, context fill and window) and the transient launch intent.
- **The project's state is a struct the situation CONTAINS**, not a second thing carried beside it.
  What is claimable describes the project rather than the agent, but what an agent may do depends on
  it — so it sits inside the one thing a caller passes. Containment makes the sharing structural: a
  project is gathered once and every agent's situation refers to that reading, where a per-agent
  re-read would turn one query into one per agent on every sweep.
- **One surface derives the allowed actions from it** — assign work, nudge, compact, clear, reclaim
  the pod, wake it — each carrying the reason it is unavailable, "" meaning allowed. This mirrors
  `Offered.Blocked` rather than inventing a second convention, because those reasons are what an
  agent is told and a user reads.
- **The gathering is centralised with it.** The hub already caches everything expensive: the
  watchdog holds liveness, fill, model and the pod listing in memory, and the roster and state are
  local store reads the board already makes per agent. So the surface gathers for itself and callers
  need assemble nothing.
- **The 23 sites ask the surface** instead of re-deriving. `agentBlocked`, `HoldsNothing`,
  `AtLeafBoundary` and `Stalled` become fields of it rather than separate functions with separate
  inputs.
- **Front-ends read the answer, never recompute it.** A capability a front-end needs travels on
  `AgentView` as a field the hub has already decided, the way `Status` and `Retired` do. This is
  forced rather than chosen: `importguard_test.go` forbids a front-end linking hub code, so a rule
  the hub evaluates cannot be evaluated again in the TUI without duplicating it.

## What does not change

- `registry.Surface` keeps its job — what an agent may type is a different menu from what the hub
  may do, and both stay. What they share is the situation, so `Caller` is derived from it rather
  than assembled separately.
- No rule changes meaning here. This moves where rules live and how they are asked; a rule that is
  wrong today is wrong afterwards, in one place instead of several.

## Why it is worth the churn

Three defects this month came from one rule living in several places: the retired-agent nudge above,
`SetState` writing whole rows so three callers silently dropped a held container, and a launching
state whose two tickets contradicted each other because nothing agreed on what "launching" meant.

An arch test holds the line afterwards. `internal/arch` already does this for two invariants —
who may inject into a session, and who may poll the container runtime — by enumerating call sites
and failing the build on an undeclared one. Without it a sixth decider appears within the month,
which is precisely how the fifth did.
