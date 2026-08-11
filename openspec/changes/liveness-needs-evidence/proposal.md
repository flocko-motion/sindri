# A stale listing is not evidence that an agent is down

## Why

A freshly launched agent read `down` on the board while it was demonstrably running. Observed on
nidi: the row said down, `agent info nidi --debug` reported both live probes passing, and the agent
went on to submit its PR from that state.

`watchdog.sweep()` read the pod list from `container.ListByLabelCached` and treated absence from
that MEMOIZED listing as a settled verdict — recorded at once, no strikes, no probe. A container
created after the memo was filled is not in it. nidi's pod was created at 12:54:47 by a relaunch;
the listing predated it.

The asymmetry is the actual defect. A failed probe is deliberately only a strike — `downStrikes`
is 3, because "one contended exec is not evidence". The listing path bypassed that entirely and
issued the strongest verdict available from the weakest evidence. The board reads the watchdog's
last observation, so the row stayed wrong until the memo aged out and a sweep re-listed. It
self-corrects, which is why the agent kept working, but the window is exactly when a human is
watching a launch.

The same mistake sat in a second place. `state.go` built liveness into a `[]bool`, so an agent the
watchdog had never observed arrived as `running=false` — a zero value reading as a negative
observation.

## What changes

- The sweep lists uncached. It is the one caller whose question is about NOW; the memo exists for
  the orphan scan in `State`, which runs per board read. `ListByLabelFresh` also primes the memo,
  so the board reads that follow are answered from the newer result rather than the one it
  overtook.
- Absence from a listing becomes a strike like any other failure. No single reading settles
  anything, whatever its source, so neither a lost probe nor a listing taken a moment ago can
  declare an agent down alone. This removes the "conclusive" class rather than narrowing it.
- An agent with no observation at all no longer reads `down`. `down` is a claim, and nothing has
  looked. It reads `unknown` until the next sweep answers, and an unobserved reading no longer
  retires a launch or stop intent as fulfilled.

## Impact

- Specs: `hub` gains a requirement for how liveness is decided. It was unspecified.
- Code: `internal/container/runtime.go` (`ListByLabelFresh`), `internal/hub/watchdog.go`
  (fresh listing, absence as a strike, the `conclusive` parameter dropped),
  `internal/hub/agent/lifecycle.go` (`AgentStatus` takes whether the agent was observed) and
  `internal/hub/state.go` (carrying that through).
- A deliberate cost: a genuinely stopped agent now reads up for `downStrikes` sweeps rather than
  one, about six seconds. That is the hysteresis a failed probe already had, and it errs toward
  the error that self-corrects — a stale "up" is fixed by the next sweep, whereas a false "down"
  is read by a human and acted on.
- `unknown` is a status word the front-ends had not seen. They render the hub's word as given, so
  neither needed a change; the two stale enumerations in their doc comments were corrected, both
  of which already omitted `blocked`, `stalled`, `full`, `launching` and `stopping`.
