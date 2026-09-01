# Tasks

## 1. The situation

- [x] 1.1 One struct carrying an agent's whole situation: roster row (role, retired, clear-armed,
      stopped), workflow state (phase, task, container, escalation), the observer's reading (up,
      runtime, clients, still-for, fill, window), the launch intent, and what it could be handed.
- [x] 1.2 It gathers for itself, taking only the agent's identity.
- [x] 1.3 The per-project half — the claimable pool — is read once and shared when a whole roster is
      judged, never per agent. Pin it with a test that counts the reads. The test asserts the shared
      pool's IDENTITY rather than a query count: the store has no seam to count through, and identity
      proves one read was shared where equal copies would not.
- [x] 1.4 No container operation, whatever the state. A test asserts it, over the package's imports
      (`internal/arch`: TestASituationCostsNoRuntimeCall).

## 2. The surface

- [x] 2.1 One derivation from a situation to the allowed actions: assign, nudge, compact, clear,
      reclaim, wake, plus stalled and needs-user.
- [x] 2.2 Each action carries the reason it is unavailable, "" meaning allowed — the shape
      `Offered.Blocked` already uses, and the words an agent is actually told.
- [x] 2.3 The reasons are the ones agents already receive, so no refusal changes wording by accident.
      Two are new, because the sites they replace composed no words at all: the leaf-boundary refusal
      (its old wording died with `FireClear`) and the reclaim refusals, which were bare booleans.

## 3. Move the rules in

- [x] 3.1 `agentBlocked`, `HoldsNothing`, `AtLeafBoundary` and `Stalled` become fields of the surface
      rather than functions with their own inputs. `WakeRefusal`, `parkedByTheHub` and
      `reviewerAssignable` went the same way; the first two keep their names as the delivery path's
      own words for `Surface.Wake`.
- [x] 3.2 The 23 sites across 11 files ask the surface. `workflow/task.go` holds 6 of them and
      `workflow/explain.go` 3.
- [x] 3.3 `AssignPendingWork` gains the guard it never had — the bug that prompted this
      (-> sd-daf9c7), which falls out rather than being fixed separately.
- [x] 3.4 `registry.Caller` is derived from the situation instead of assembled separately, so the two
      menus cannot disagree about the state they share.

## 4. The front-ends read, never repeat

- [x] 4.1 A capability a front-end needs travels on the board as a decided field
      (`api.AgentView.NeedsUser`).
- [x] 4.2 No front-end applies one of these rules itself: the three `api.AgentNeedsUser` call sites in
      the CLI and TUI read the field, and `CountAgentsNeedingUser` sums it.

## 5. Hold the line

- [x] 5.1 An arch test in the manner of the injection and runtime-monopoly guards: the call sites are
      enumerated, and one deriving a rule the surface owns fails the build with where it belongs
      (`internal/arch`: TestTheSurfaceIsTheOnlyHomeForTheseRules).
- [x] 5.2 The test's allowlist names each exception with its reason, so the argument is on record.

## 6. Landing

- [ ] 6.1 ONE agent does the whole change, with the rest of the fleet retired AND STOPPED. 6 of the
      23 sites are in `workflow/task.go` and several agents edit it daily, so anything else in flight
      rebases over this or it over them.
- [ ] 6.2 Stopped as well as retired, until sd-daf9c7 lands: `AssignPendingWork` does not check
      retirement, so a retired agent still running takes a "ready for you" push every ~30s. A stopped
      one is not nudged (the guard needs it up).
- [ ] 6.3 Submit per section rather than once at the end. Sections 1-2 (situation and surface), 3
      (move the rules in), 4 (front-ends), 5 (the arch test) each stand alone and are reviewable on
      their own — and each landing gives the agent a fresh window for the next, where 23 sites in one
      session would not.

      NOT POSSIBLE AS WRITTEN: the deadcode linter fails a surface nothing asks yet, so sections 1-2
      cannot land before 3. 1-3 go together; 4 and 5 could have followed separately.
