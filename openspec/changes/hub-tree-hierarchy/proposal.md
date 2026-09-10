## Why

`internal/hub/` holds fifteen subdirectories at its root, and six loose files driving the hub's
clock. Three groups already exist — `api/`, `messaging/`, `flow/` — and everything that did not fit
one of them stayed at the top, so the root reads as a list rather than a structure. Two groupings
are missing: the packages that describe what the hub KNOWS, and the work it does on a TIMER.

## What Changes

- `internal/hub/world/` groups what the hub knows as data: `store`, `situation`, `observe`, `task`,
  `owned`. Each is a leaf many packages read and almost nothing reads back — `situation` alone has
  twelve importers across five trees, which is why it has no single owner to nest under.
- Two further moves were attempted and reverted, each refused by an invariant already in the tree
  (both written up in design.md):
  - `internal/hub/prompts` under `flow/` — `flow` combines the modules, so it sits above them;
    nesting a leaf that `messaging/mail` reads there runs the dependency backwards, which
    `TestDeliveryDoesNotAskTheRuleset` exists to refuse. `prompts` stays a root leaf.
  - `internal/hub/sweep/` for the six clock files — the package extracts cleanly, but its 685
    lines of test drive the watchdog's mutex and raw observation map over a fully built hub, and
    `sweep` cannot import `hub`. The watchdog is the composition root's polling organ.

Every move keeps its package's own identity — these are directories gaining a parent, never files
being merged. No behaviour changes.

## Capabilities

### New Capabilities

- `hub-tree-shape`: what belongs at the hub's root, what earns a grouping directory, and the rule
  that decides which parent a package nests under.

### Modified Capabilities

<!-- None: every package keeps its behaviour and its API; only its import path changes. -->

## Impact

- **Moved, no behaviour change**: `hub/{store,situation,observe,task,owned}` → `hub/world/…`;
  `hub/prompts` → `hub/flow/prompts`; `hub/{ticks,tick_*,watchdog,watchdog_status}.go` →
  `hub/sweep/`. `hub/store` is imported by 100+ files, so the bulk of the diff is import paths.
- **One real seam**: the sweeps and the watchdog are package `hub` today and reach fifteen of its
  internals. They need a `sweep.Deps` interface, the way `frontend.Hub` was cut — the only part of
  this change that is more than a relocation.
- **Two guards updated**: `flowMayImport` gains the new `situation` and `observe` paths and learns
  to skip a nested vocabulary directory; `internal/arch/situation_test.go` follows `situation` to
  its new path so the no-runtime-call rule keeps applying.
- **Out of scope**: `hub/core`, `hub/project`, `hub/comments`, `hub/sections`, `hub/agent`,
  `hub/flowtest`. Each is either a seam the composition root owns or a subsystem in its own right,
  and none has a parent that would be true for all its callers.
