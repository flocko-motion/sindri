# Tasks

## 1. Remove the cause: the sweep asks about now

- [x] 1.1 `container.ListByLabelFresh` lists without consulting the memo and primes it with the
      result, so the board reads that follow use the newer answer.
- [x] 1.2 The watchdog's seed and sweep both use it. They are the callers whose question is about
      the present moment; the memo stays for the orphan scan, which runs per board read.

## 2. Remove the class: no reading stands alone

- [x] 2.1 Absence from a listing records as an ordinary failure, so it accumulates strikes like a
      lost probe instead of settling the matter.
- [x] 2.2 Drop the `conclusive` parameter rather than leaving it always-false, and rewrite the doc
      comments that promised the old behaviour.

## 3. "Not yet observed" is not "down"

- [x] 3.1 `AgentStatus` takes whether the agent was observed at all, and answers `unknown` where
      nothing has looked.
- [x] 3.2 A non-observation no longer retires a launch or stop intent as fulfilled.
- [x] 3.3 `state.go` carries the distinction instead of letting a `[]bool` zero value speak. A
      board read still never probes — that constraint is what made carrying it the only option.
- [x] 3.4 Correct the two stale status enumerations in the `AgentView` and Agents-tab doc comments.

## 4. Pin it

- [x] 4.1 A fresh list sees a pod created after the memo was filled, and primes the memo with it.
- [x] 4.2 One missed listing holds the agent up with its last good detail; the threshold reports
      down. Replaces the test that asserted the old conclusive rule, stating why it was wrong.
- [x] 4.3 Unobserved reads `unknown`, observed-and-absent still reads `down`, an intent survives a
      non-observation, and `launching` still outranks `unknown`.

## 5. The word reaches callers that decide, not only render

- [x] 5.1 `api.AgentNotUp` and `api.AgentNeedsLaunch` hold the enumeration once, where both
      front-ends already read. The two predicates differ over a launch in flight: it is not ready,
      and it must not be launched a second time.
- [x] 5.2 `ensureCoauthorAlive` launches on it and its readiness loop keeps waiting — the flow that
      broke, since a coauthor is unobserved for the seconds right after it is created.
- [x] 5.3 `agent attach`, the TUI's three attach guards, and the start/stop toggle. One shared
      `attachRefusal` so the three tabs cannot drift, wording not-yet-observed as itself rather
      than telling the user to start an agent that may already be running.
- [x] 5.4 Tests at each site, and a spec scenario that a caller may not read it as running.
