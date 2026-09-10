## Context

`internal/hub/` root, today: fifteen directories and six loose clock files.

```
agent  api  comments  core  flow  flowtest  messaging  observe  owned  project
prompts  sections  situation  store  task
ticks.go  tick_credentials.go  tick_reference.go  tick_stalled.go
watchdog.go  watchdog_status.go
```

Three of those directories are groups (`api/`, `messaging/`, `flow/`). The rest arrived one at a
time and stayed where they landed, so the root reads as a list of twenty-one things rather than as
a structure a reader can walk.

The two candidates that came up — "shouldn't `prompts` be under `agent` or `flow`", "shouldn't
`situation` be part of `agent`" — are the same question, and the importer counts answer it
differently in each case.

| package | importers, by tree |
| --- | --- |
| `prompts` (33) | flow/pr 12, flow/task 5, flow/fleet 4, flow/agent 3, flow/agent/workspace 2, flow/run 2 · **28 under flow/** — then messaging/mail 2, hub 2, agent 1 |
| `situation` (12) | flow/fleet 3, flow/pr 1, flow/agent 1, flow 1 · **6 under flow/** — then hub 2, agent 2, core 1, flowtest 1 |

## Goals / Non-Goals

**Goals:**

- A hub root a reader can walk: groups, not a list.
- Every nesting justified by who imports the package, and true for all of its callers.
- The hub's clock-driven work in one directory, with a seam of its own — attempted, and refused
  by the tree; see Discovered While Implementing.

**Non-Goals:**

- Merging any packages. Every one keeps its package clause, its tests and its API.
- Changing behaviour. This is import paths and one extracted interface.
- Rehoming `core`, `project`, `comments`, `sections`, `agent` or `flowtest`. Each is a seam the
  composition root owns or a subsystem in its own right, and none has a parent true for every
  caller.

## Decisions

### The rule: nest by dependency, group by kind

> Nest X under P when P owns X or is its dominant consumer, and P's name is true for every caller.
> Otherwise group X with its peers under a directory named for what they ARE.

Nesting is a claim readers trust. `hub/agent/prompts` says "the agent package's prompts", and a
reader chasing the fyi help from `messaging/mail` would be wrong-footed by it. Subject matter
decides nothing here: most of this tree is about agents.

### `hub/world/` — what the hub knows, as data

```
hub/world/store/       the rows                              ← hub/store
hub/world/situation/   where an agent stands, gathered once  ← hub/situation
hub/world/observe/     what the harness saw of one box       ← hub/observe
hub/world/task/        the id scheme, the order, commit messages ← hub/task
hub/world/owned/       sindri's own task source              ← hub/owned
```

These five are leaves: read by many, reading almost nothing. `situation` belongs here rather than
under `flow/` because only half its importers are in that tree — and it lands beside the two
packages it is assembled from, `store` and `observe`, which is where a reader would look for it.

*Alternative considered:* `hub/situation` under `flow/`, since `flow.World` embeds it. Rejected on
the count: `hub/agent/sleep.go` and `state.go` read it too, and nesting would tell them they are
reaching into the flow tree.

*Alternative considered:* naming it `model/`. `world/` is the repo's own word — `flow.World` is
"everything an agent's conditions may read, gathered ONCE per pass", and these are the packages it
is gathered from.

### `hub/flow/prompts` — PROPOSED, then refused (-> Discovered While Implementing)

28 of 33 importers are already under `flow/`. The five that are not reach it by its new path.

The weak spot, stated: `hub/agent/lifecycle.go` uses it for the durable system prompt an agent
boots with, which is the most fundamental thing in the package and is not the flow's. 28-to-5
outweighs it.

**This does not let a state map write prose.** What stops that is `flowMayImport`, which lists what
a map may reach — and `prompts` stays off it. A map names a speech identity (`flow/agent/says`),
and `fleet/flowsay.go` resolves that to words. Nesting changes the path, never the permission.

### `hub/sweep/` — PROPOSED, then refused (-> Discovered While Implementing)

```
hub/sweep/sweep.go        the table: what runs, how often, and at startup  ← ticks.go
hub/sweep/credentials.go  carry host credentials into running pods         ← tick_credentials.go
hub/sweep/reference.go    reference-branch drift and PR health             ← tick_reference.go
hub/sweep/stalled.go      nudge an agent that stopped mid-assignment       ← tick_stalled.go
hub/sweep/watchdog.go     the one thing that polls the fleet               ← watchdog.go
hub/sweep/status.go       the derived-status diff, a tail call of the above ← watchdog_status.go
```

This is the only part that is more than a relocation. The six files are package `hub` and reach
fifteen of its internals, so they need a `sweep.Deps` the way the routes needed `frontend.Hub`.
Most of it is already exported — `PRFlow`, `TaskFlow`, `AgentFlow`, `Agents`, `Projects`, `Chat`,
`Mail`, `Fleet` were exported for the front-end — leaving `Store`, `Sit`, `Lifetime`, `observed`,
`container`, `status` and `repoDocState` to add.

`sweep`, not `upkeep` or `clock`: it is the repo's own noun for this work, in `ticks.go`'s header
and in `watchdog.sweep`.

*Alternative considered:* leaving the watchdog in package `hub` and moving only the ticks. It is
the largest single file at the root (495 lines) and the one that defines what a sweep is; leaving
it behind would put the table in one place and the biggest entry in another.

### The guard learns about nested vocabulary — PROPOSED, then reverted with the move

`TestTheDeciderCanReachNothingThatWrites` walks `internal/hub/flow` and applies `flowMayImport` to
every non-acting file. `flow/prompts` would be judged a state map and fail on `brokkr/lint`.

The fix is a `vocabularyDirs` set the walk skips, beside the existing `isActing` — the guard is
about maps and conditions, not about every file that happens to live under `flow/`. Widening
`flowMayImport` to admit `brokkr/lint` instead would put the linter in the decider's reach, which
is the opposite of what that list is for.

## Discovered While Implementing

Two of the three moves were tried and reverted. Both refusals came from the tree itself, and both
are worth more than the moves would have been.

### `prompts` does not go under `flow/` — the layering forbids it

The count said yes: 28 of 33 importers are in that tree. Two guards said no, and they were right.
`flow` COMBINES the modules, so it sits above them; `messaging/mail` is a module, and
`TestDeliveryDoesNotAskTheRuleset` exists to keep mail from depending on the ruleset. Nesting
`prompts` under `flow/` makes `mail -> flow`, which is the dependency running backwards.

`prompts` is a leaf both layers read, so it belongs BELOW flow, and a leaf at the hub root is
reachable from every layer without claiming ownership. The first guard I hit
(`TestTheDeciderCanReachNothingThatWrites`) I bent with a `vocabularyDirs` skip; the second showed
that bending was the wrong instinct. `internal/hub/flow` appears 41 times across the arch tests as
a path-keyed boundary meaning "the decider" — a shared vocabulary package inside it costs an
exception at each, and every exception makes the boundary mean less.

### `hub/sweep/` is not extractable, and the reason is the point

The six clock files moved cleanly and the package compiled behind a `Deps` of sixteen methods. The
tests are what refused it: 685 lines that reach `w.mu`, `w.obs` (the raw observation map),
`w.record`, `w.capacity` and `status.sweep`.

Attempted twice. The first refusal was reasoned from a count and was too quick — the repo already
has `internal/hub/flowtest` for exactly this, so a fake `Deps` was the obvious answer and it worked:
a ~110-line fixture over a real store and a real agent service, and every sweep test but four ran
green inside the package.

The four that would not move are board assertions — they check what `hub.State` renders from a
GIVEN reading, so they need the hub AND control of the observer. Serving them meant exporting a
test-only surface on `Sweeps`: `Still`, `Seed`, `SeedFill`, `SweepNow`, `StandStill`,
`SeedCapacity`, `SeedPods`. Seven functions no production caller ever reaches.

Two linters refused that, correctly and independently. `deadcode` wants each marked
`//deadcode:keep` — a directive with no other use anywhere in this repo. `comment-length` then
fails the file outright: a documented exported function plus a directive is three comment lines
minimum, against a 1.5-line average target, and no amount of trimming reaches it. Merging the seven
into fewer, larger functions makes the average worse, not better.

That is the finding, and it is sharper than "the tests need a hub": extracting the observer forces
a test-only API onto a production type, because the hub's tests and the observer's tests both have
to reach inside it. Two independent guards say the same thing. Six well-named files in the
composition root describe that better than a package pretending to be independent.

The finding underneath: the watchdog IS the hub's polling organ. It reaches fifteen of the hub's
internals because it observes all of them, and the sweeps are scheduling around what it sees. Six
well-named files in the composition root describe that better than a package pretending to be
independent. `ticks.go` alone could move, but a "sweep" directory holding the table and not the
biggest sweep is worse than neither.

## Risks / Trade-offs

- **`hub/store` has 100+ importers, so the diff is enormous and mostly mechanical.** → One
  path-and-qualifier rewrite per package, each verified by enumerating the match set first, with
  `go build`, the full suite and all ten linters after each. The `header-path` linter added last
  change catches any header left behind.
- **Extracting `sweep` touches the hub's own startup order**, which is commented as load-bearing
  (`agentCh` before `agents`, `sit` before both, `watch` before `status`). → `Deps` is read at call
  time, as `frontend.Hub` is, so construction order is unchanged; `hub.open` still builds
  everything in the order it does now.
- **`world/` could become a dumping ground** for anything that fits nowhere. → Its rule is stated
  in the spec: read by many, reading almost nothing. A package that writes does not qualify, which
  is why the watchdog — the one thing that polls — is in `sweep/` instead.
- **Five nested packages mean five `// package:` headers to update.** → The `header-path` linter
  fails the build on any that is missed.

## Migration Plan

Four commits, each green on `go build`, the full suite and all ten linters:

1. `hub/world/` — the five data leaves, their headers, and the two arch guards that name
   `internal/hub/situation` and `internal/hub/observe` by path.
2. `hub/flow/prompts` — the move, plus `vocabularyDirs` in the flow purity guard.
3. `hub/sweep/` — `Deps`, the six files, and the hub's accessors for what it does not yet expose.
4. Close out: `brokkr map internal/hub` read end to end, checking every header names its new home.

Rollback is per commit; 1, 2 and 4 are reversible moves, and 3 is one extracted interface.
