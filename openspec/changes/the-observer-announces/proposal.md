# The observer announces a change, and the orchestrator listens

## Why

The harness seam landed: one `Observation` crosses it whole, and `Clear`/`Compact`/`SetModel` block
and answer. The abstraction is there. **Nobody uses it to tell the orchestrator anything.**

The observer watches continuously and unprompted — `watchdog.loop()` beats every second and probes
each agent every sixth — and `record()` already holds the previous reading beside the new one, which
is what the pane `digest` is for. `statuswatch.sweep()` diffs the derived status word on top.

So the hub knows, within seconds, that an agent went idle, stopped moving, or fell over. It tells:

- the in-memory `obs` map
- a `state_log` debug row
- the board's SSE stream, so the TUI redraws

It does not tell the component that decides what to DO about any of it. The orchestrator finds out
by asking on a different clock, or by waiting for the agent to run `sindri` and ask for itself.

**Every timer in the hub is a workaround for that missing hop.** `ReannounceAfter` waits five
minutes to re-announce mail because nothing says "this agent is reachable now". `stallwatch` runs
its own ticker to ask a question the observer could have answered at the moment it looked. Mail
delivery is best-effort into a session whose readiness was sampled by somebody else, on another
schedule. Each is a poll standing in for an event that already exists and is thrown away.

It is also why an agent's state is discovered LATE rather than WHEN it changes: an agent that goes
idle holding nothing waits out a stall interval instead of being noticed on the beat that saw it.

## What Changes

**`record()` publishes when a settled reading differs from the last.** It is one funnel — every
observation in the system lands there, and the comparison is already inside it — so this is one call
in one place rather than a new mechanism or a second source of truth. The observer monopoly is what
makes it safe: exactly one thing looks, so exactly one thing announces.

**On the SETTLED reading, never on every beat.** A one-second beat driving rule evaluation would
re-decide continuously on a flapping agent. `strikes` (which already holds `up` until
`downStrikes` consecutive failures) and `stillSince` are the existing dampening, and what a
subscriber must see: the reading the hub is prepared to stand behind, not the raw sample.

**The orchestrator subscribes.** A change in an agent's observation is delivered to the rules that
care, carrying the previous reading and the new one — a subscriber's first question is almost always
*what changed*, and re-deriving that from the store is how two components come to disagree.

**The pollers become subscribers.** `stallwatch`'s ticker, the mail re-announcement's five-minute
timer, and every "ask again in a while" that exists because nothing announced. Each is deleted or
reduced to a backstop, and its interval stops being a tuning parameter for something that should
have been an event.

**Delivery reads the readiness the observer just published**, rather than sampling it again on its
own schedule — the mail-push race that produced "pushed: true" over a pane that never showed the
text.

## What does not change

- The observer still POLLS the runtime. tmux and podman push nothing, so somebody must look; the
  point is that the looking already happens and its result stops at a screen.
- No new source of truth. Subscribers receive the same `Observation` `Observe` already returns.
- A subscriber that is slow or fails must not stall the observer — the sweep is the one thing the
  whole hub's liveness depends on. Announcing cannot become a place the beat can block.

## Impact

- Specs: `agent-runtime` gains the announcement; `05-workflow` gains what the orchestrator does with
  it and what stops being timed.
- Code: `internal/hub/watchdog.go` (`record`, `recordFill`), `internal/hub/statuswatch.go` (which
  already diffs and only logs), `internal/hub/stallwatch.go`, `internal/hub/workflow/mailnudge.go`,
  and the `Harness` port.

## Where this sits

Directly on top of the archived `the-harness-reports-observations`, which made the observation a
value that crosses the seam whole. This is the other half of the same idea: the harness saying WHEN
it changes, rather than only answering what it is when asked.
