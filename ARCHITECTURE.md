# Architecture

Sindri follows a **strict hexagonal architecture** (ports & adapters). These rules
are not aspirational. Several are enforced by tests that fail the build (each named
below where it applies); the rest are enforced in review. A rule you can break
without a test failing is still a rule.

## The core is headless

All domain logic lives in the core (`internal/hub` and the packages it uses) and
is **headless and interface-agnostic**. It knows nothing about how it is invoked:
no terminal, no key handling, no rendering, no flags. It exposes operations; it
does not care who calls them.

## The outside world is reached only through adapters

Every interaction with something outside the process — git, podman, tmux, GitHub,
the spec tool — goes through an adapter in `internal/adapter/`. The core calls
adapters; it never shells out, dials a socket, or touches an external tool directly.

**Name the port, not the tool.** Where a family of implementations exists for one
job — the container runtime, the coding agent, the task source — the core depends on
the **port**, the interface naming that job, and never names a concrete adapter.
Choosing the implementation is the composition root's work: the core is handed one
and never constructs it.

Where a tool has exactly one implementation and no plausible second — git, tmux —
the core imports its adapter directly. The abstraction earns its place by making an
implementation swappable or fakeable; requiring one where nothing can vary buys
nothing and hides which tool is in use.

**A port hides its differences, or it is not a port.** Sindri's own tasks, openspec
changes and GitHub issues are all tasks: each is worked, closed, submitted and
reviewed identically, and no caller may branch on which kind it holds. Where the
kinds genuinely differ — a task sindri owns keeps its status in the hub's store, a
mirrored one keeps it at its source — that difference lives behind one operation
(`hub/flow/pr`'s `SetStatus`) and stops there. A caller that has to ask what kind of
task it has is a caller that will one day forget to; four of them did, each leaving a
finished openspec change reading open. `internal/hub/flow/fleet/onehome_test.go` holds
the line for status specifically: only the owned source may write `owned_tasks`.

## CLI and TUI are interchangeable front-ends

The CLI and the TUI (both under `internal/ui`, launched by `cmd/sindri`) are **thin
interface layers**.
They must always execute the **same** core logic — they translate user intent into
core operations and render the result. Nothing more.

- **No business logic in the CLI or TUI.** Only interface logic belongs there:
  argument parsing, key handling, layout, rendering, formatting.
- Any behaviour offered by one front-end must be reachable from the other, because
  both drive the same operations. `internal/ui/parity_test.go` enforces it over the
  client's method set; a deliberate exception goes in that test's allowlist with the
  reason it is not a gap, so the argument is on record rather than assumed.
- A front-end reaches the core through the client (`internal/client`), which talks
  to the single hub — so the CLI and TUI are literally running the same code.
- **A front-end links no hub code.** It carries the exchange format (`internal/api`)
  and the client, and no hub package, persistence driver, or adapter that only the hub
  needs. An adapter the front-end itself needs is fine — it attaches to tmux and runs
  git locally. A front-end that needs a hub running starts it by executing that
  binary, rather than constructing one in its own process.
  `internal/ui/importguard_test.go` enforces the `internal/hub` half of this over the
  real import graph, so an indirect import fails too.

## Every file declares its layer

Each non-test `.go` file opens with the four-field header `brokkr map` reads, and its
`type:` names the layer from a **closed set**: `logic`, `adapter`, `assembly`,
`rendering`, `ui`, `command`, `entrypoint`. Closed on purpose — a vocabulary anyone
may extend describes nothing, and a file that fits none of the seven is usually a
file doing two jobs. `internal/arch/vocab_test.go` fails the build on an eighth.

## The workflow is a reconciler, in three layers

What an agent should be doing next is decided by a **level-triggered reconciler**, split so that
deciding and acting cannot be the same act.

- `internal/hub/flow/machine` is the **engine**, and it names nothing of sindri's. It owns the loop: one
  action in flight per subject with its own `ctx`, notifies coalesced while that action runs,
  cancel-and-restart when a fresh decision disagrees, a periodic floor under every dropped event, and
  the record of each pass under one correlation id. It is tested against a fake world and intent.
- `internal/hub/flow` and the declarations under `flow/agent/roles` are the **flow**: the states a
  subject can stand in, each DECLARED as a struct that reads as its own documentation — title, what
  it means, every way out with the condition that takes it — plus the world they read and the closed
  vocabularies they answer in (`flow/agent/act`, `cond`, `says`, `topic`). Each role has its own map
  over a set of shared states; they add up to one registry, because the subject is an agent and not a
  role. `internal/arch/flow_test.go` fails the build if the declaring half can reach anything that
  writes, so deciding is pure by construction rather than by intention.
- The **acting halves** gather the world and perform what the maps decide, one package per subject:
  `flow/pr`, `flow/task`, `flow/run`, and `flow/agent` for agents themselves. Each is a map beside the
  code that acts on what the map decides, and the line is drawn per FILE — anything named `*_act.go`
  is the acting half and is the only place that writes state or touches the harness.
  `internal/hub/flow/fleet` holds the four running machines and is what every acting half re-decides
  through (`core.Flows`).

### Everything from outside is a signal; only the machine changes state

Every way the world reaches the hub is a **signal** — a user action, an API call, a verb an agent
types, an event from a task source. A signal does one of two things and nothing else: it **sets a
flag**, or it **publishes a message**. The flow machine is what changes state, and it does so by
**observing** signals against the world they left behind.

So no caller moves a subject, and no caller decides which agent a piece of work goes to. Anything
that hands an agent work, takes it away, prepares its session or moves its pod happens inside the
machine, through the state that subject is standing in.

Signals come in two kinds, and the difference is what may be relied on:

- A **durable** signal is a flag or a record the machine can re-read whenever it looks — `retired`,
  `stopped`, a clear armed, a merge intent, a verdict on a review row. All the meaning lives here.
- A **transient** signal is a message published on a topic (`internal/hub/flow/topic`). It carries no
  payload and no authority; it says only *look sooner*.

Where a human's request is a lasting condition — retired, stopped, a clear armed — the flag *is* the
request. Where it is an instruction to act and the resulting state already holds — "restart this
pod", which is already up — it is a record carrying the time it was asked for, so that *asked for,
and not yet done* is a signal the machine can observe.

**A message carries no meaning.** It is a prompt to go and look, and the answer comes from the whole
world. A dropped message costs latency and never correctness, ordering stops mattering, and a
cancelled transition needs no compensation — the machine re-decides from wherever it landed. Nothing
here builds reliable delivery, ordering or replay; all three are edge-triggered thinking. The test is
blunt: **delete every publication and the system still reaches the same states, more slowly.**
Anything that strands without its message is a decision that was riding in one.

This is what one of the four machines was missing, and what it cost is on record: a reviewer's map
was walked by the beat while a second path claimed reviews of its own, and together they handed a
pull request to an agent whose pod was down, where it sat unread.

What an agent **holds** and where it **stands** are two facts with two writers. A hand-over or a
release records the first (`store.SetHolding`); the machine alone writes the second
(`store.SetPhase`). One call wrote both until twenty-eight places moved an agent without deciding
anything, and splitting them is what makes "one writer" expressible at all.
`internal/arch/onewriter_test.go` counts the callers of each act upon an agent and fails on a second
— by call site over the product, with no allowlist.

A test reaches a state through **one door** (`flowtest.Place`), which writes both facts through both
writers and refuses a state no role's flow declares. A fixture whose subject IS the flow — what a
verb, a verdict or a merge leaves an agent standing in — raises the signal and runs the machine,
because a fixture that writes the answer it is asserting proves nothing.

A pass is **atomic per subject**. Every look used to happen on the loop's one goroutine, which stopped
being true when a caller could ask for one; two passes over an agent then each decided from the state
before the other moved it. A caller waits its turn, and the loop skips a subject somebody is mid-pass
on — the same bargain a dropped message makes.

### Every wait is a state, and transitions are instant

Wherever the hub asks for something and time passes before the answer, the subject **stands in a
state** for the duration, and that state's action does the work under a `ctx` the machine cancels on
the way out. A transition takes no time and carries no work: the process belongs to the state.

So a pod coming up is `launching`, a pod going away is `stopping`, a session being discarded is
`clearing`, a merge in flight is `merging`, and a submit's questions are a state the agent sits in
while it researches. The test is whether an event can arrive in the middle, which in practice means
whether there is a `ctx` worth cancelling. No directive may mean "a state I have no name for", and no
wait may live inside a verb, a sweep, or an observer's bookkeeping.

A state's action may run for as long as it likes and may hold a conversation, because cancellation is
the cleanup: whatever it was doing is no longer what the subject is here for, and the cancel is
recorded rather than silent.

### A repair is a condition, not a sweep

A world in a shape that should not exist is corrected by a condition on the subject that holds the
mismatch, watched wherever that subject stands — which is what `cond.TaskGone`, `cond.FeatureGone`
and `cond.ReviewOvertaken` already are. No sweep exists to repair state, and no repair is reachable
only at startup: a fix that can only happen at boot cannot happen when the shape appears.

What is left on a beat is the **task sources**, which announce nothing: td, openspec and GitHub are
polled, so the cached read model only moves when something reads them. That is the honest reason a
sweep exists at all (`internal/hub/tick_reference.go`).

### A shared state is built once

Every role carries the full session and pod lifecycle, however few work states it has — a role the
machine cannot reach for a clear or a launch does not exist. WHERE a lifecycle state leads differs by
role; WHAT can move a subject out of it does not, so the event list is written once in
`flow/agent/roles/lifecycle` and each role passes only its own destinations, positionally, so a role
that omits one fails to compile. The bug behind that shape was a single missing edge in one of two
hand-written copies.

## Context is handed through, never invented

`context.Background()` belongs at an **entrypoint** — a `main`, a server's request
root, a background loop where it starts. Everywhere below that, a function takes the
caller's context and passes it on, narrowing it where the work needs its own bound:
`context.WithTimeout(ctx, probeTimeout)` around a probe, never a fresh root.

Inventing one at depth cuts two wires at once. Cancellation stops reaching the work,
so an operation the caller has abandoned runs on; and a root carries no deadline to
inherit, so a wedged dependency blocks its caller for ever. Both have happened here —
a liveness probe on `context.Background()` held every board read open until podman
answered, which under load it did not.

In the hub this resolves into three lineages, and every call belongs to one of them.
A **read** takes the request's context, so abandoning the request abandons the work.
Work the hub must **finish** whatever the client then does — a pod coming up, a pod
going away, a message landing — takes that context with its cancellation dropped
(`hub.detached`), which keeps the lineage visible at the handler where the decision
belongs. **Fleet-side** work the hub does on its own account runs under the hub's
**lifetime** (`Hub.lifetime`, cut from the root `main` hands to `hub.New`): its loops,
and the pushes it makes through a port whose signature carries no context of its own.

`internal/arch/context_test.go` holds the line: a file that starts a context must be on
its list with the reason it is an entrypoint, and an entry that stops rooting anything
has to go — so the argument stays on record rather than being remembered.

## Topology

There is **one hub per machine** — the single global coordinator that owns all
state and drives every agent across every repo. Agents run in **podman pods**, one
per agent, isolated in their own git worktrees. Front-ends and agents reach the hub
over its unix socket (over TCP on macOS, where the podman VM can't cross a
bind-mounted socket). The hub is the only process with domain logic; everything
else is a front-end or a pod.

## Project map

Read a package before you change it: `brokkr map <path>` prints every file's header
with each declaration's doc and signature, and `--grep <regex>` finds a declaration
by name or body. Every non-test file's header answers *package / type / job / limits*,
so the map below is the index and the headers are the entry.

**Binaries**

| path | what it is |
| --- | --- |
| `cmd/sindri` | the product: the host CLI and the TUI launcher |
| `cmd/sindri-hub` | the hub: the single writer, foreground or spawned by `sindri hub start` |
| `cmd/sindri-worker` | the in-pod client the agent types against |
| `cmd/brokkr` | dev tooling — code map, linters — kept **out** of the product |
| `openspec/` | specs drive behaviour changes; `openspec/changes/` holds the ones in flight |

**Deciding — pure, and it may reach nothing that writes**

| path | what it is |
| --- | --- |
| `internal/hub/flow/machine` | the engine: states, transitions, one action per subject, the pass record. Names nothing of sindri's |
| `internal/hub/flow` | the `World` every condition reads, and the shapes a map is written in |
| `internal/hub/flow/agent/roles/{worker,reviewer,planner,coauthor}` | the four maps. **Read these to know how an agent behaves** |
| `internal/hub/flow/agent/roles/lifecycle` | the states every role shares — launching, stopping, clearing, mail, escalated, retired — built once |
| `internal/hub/flow/agent/act` | the actions a state may run, as identities with their outcomes |
| `internal/hub/flow/agent/cond` | every question a map can ask about the world |
| `internal/hub/flow/agent/says` | the identities of what a state tells an agent |
| `internal/hub/flow/topic` | the closed set of announcements, and who listens for each |

**Acting — the only place that writes**

| path | what it is |
| --- | --- |
| `internal/hub/flow/fleet` | the four running machines, the actions behind every declared one (`flowdo.go`), and the world-gathering that feeds them |
| `internal/hub/flow/pr` | a pull request's own map and its acting half — review, verdict, gate, merge, scrap |
| `internal/hub/flow/task` | a task's own map and its acting half — claim, approve, plan, reconcile |
| `internal/hub/flow/run` | the fleet's single run queue: one slot, its own map |
| `internal/hub/flow/agent` | the acting half for agents — escalation, fullness, the pod, the stall, and the verbs an agent runs on its own tree (`workspace/`) |

**The world the conditions read**

| path | what it is |
| --- | --- |
| `internal/hub/world/store` | SQLite, hub-owned. Every row the hub keeps, one file per subject |
| `internal/hub/world/situation` | one agent's whole standing, gathered once — and `Surface`, the single home for what may happen to it |
| `internal/hub/world/observe` | what the harness SAW, as evidence with a timestamp. It concludes nothing |
| `internal/hub/world/task` | the Task entity over its three sources, the id scheme, and the claim order |
| `internal/hub/world/owned` | sindri's own tasks, presented as a task source like any other |

**Seams to the outside**

| path | what it is |
| --- | --- |
| `internal/hub/harness` | the pod and session seam: launch, stop, clear, attach, and the transient intent behind each |
| `internal/hub/prompts` | every word an agent is told, in one place |
| `internal/hub/messaging/{mail,chat}` | the durable mailbox, and the user's chatroom |
| `internal/hub/api/agents` | the agent-facing surface: the verb catalogue, the registry that gates it, the socket |
| `internal/hub/api/{frontend,serve}` | the front-end-facing surface, and the listener under both |
| `internal/hub/core` | the handles every subject is given, and the shared error vocabulary |
| `internal/adapter/*` | git, podman, tmux, GitHub, td, the coding agent. **Nothing else shells out** |

**Front-ends — they link no hub package**

| path | what it is |
| --- | --- |
| `internal/api` | the wire types both sides share, plus the pure board rules |
| `internal/client` | how a front-end reaches the hub |
| `internal/ui/{cli,tui}` | the two front-ends, which must stay reachable-for-reachable |
| `internal/hub/sections` | the dashboard tabs every UI renders, with their badge counts |

**Guards**

| path | what it is |
| --- | --- |
| `internal/arch` | the architecture tests. Each holds one rule from this document, with the reason on record |
| `internal/hub/flowtest` | the shared fixture every subject's tests are built from |
| `internal/brokkr/lint` | the linters `brokkr lint` runs — the quality floor |

## Conformance

All code must conform to the `brokkr` linters (`brokkr lint`, run by `make check`
— this project's declared gate, which chooses to run it). That baseline is
enforced automatically — a change that fails it does not merge.

## Why

Behaviour is defined once, testable without a terminal, and identical no matter how
it is driven. A change to what sindri *does* lands in the core; a change to how it
*looks* or is *typed* lands in a front-end.
