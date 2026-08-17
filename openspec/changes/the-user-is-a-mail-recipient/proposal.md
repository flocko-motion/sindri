# The user is a mail recipient, and an agent may write to it — sparingly

## Why

The mailbox has one kind of recipient: an agent. The user appears only as a `Sender`, which means
everything an agent notices flows one way — into a PR body, a task comment, or its own log, all of
which are read only by someone who goes looking at that particular thing.

So a fact with no home is lost. An agent that notices the task tracker is shelling out twice per
sync, or that a config field is documented backwards, has nowhere to put it: it is not about the task
in hand, not a blocker, and not part of the change under review. Nobody will ever learn it unless
that agent says so, and today it cannot.

The reason there is no such channel is sound, though: one person cannot read a fleet. Any channel
opened here has to be bounded before it is opened, not after the first flood.

## What changes

- **The user becomes a first-class recipient.** `api.Mail.Agent` may name `"user"` — the same spelling
  that already stamps a message FROM them, so the symmetry costs nothing — and no agent may be named
  it, so the two can never collide in one mailbox.
- **`fyi "<message>"`**, open to every role. The name states the register: not a request, not a
  report. One short note about something noticed in passing.
- **A budget in four named constants, in one file**, because the first values are certainly wrong and
  tuning must be a one-line change:
  - `maxNoteLen` (300 characters) — over-length is REFUSED, never truncated, since a silent cut drops
    the point and teaches nothing. The refusal says to cut it and explicitly not to split it across
    two calls, which is the obvious workaround.
  - `NotesPerClaim` (2) — granted per CLAIM, whole, however long the claim runs. Work-based rather
    than time-based: time accrues while an agent sits idle, rewarding having seen nothing, and lets a
    long-blocked agent wake with a full purse. A grant per claim ties the right to speak to having
    been somewhere and looked at something.
  - The grant **replaces, never accumulates**. Finishing with two unspent starts the next claim at
    two, not four; banking is what turns any quota into an occasional flood.
  - `fleetNotesPerHour` (6) over a rolling `fleetNoteWindow` — the ceiling that actually protects the
    user. A work-based budget scales with the fleet and one person's attention does not, so forty
    agents each behaving impeccably still bury them, every individual decision along the way correct.
- **At the ceiling it refuses rather than holds.** Refusing is honest and leaves no queue the user
  cannot see; holding preserves a note at the cost of delivering it hours stale. The cost is real — a
  valuable note can die because two chatty agents got there first — so every refusal is LOGGED, and
  those counts are the only evidence for what the ceiling should be. This is the reversible half of
  the design.
- **The agent is told what it has left**, in the verb's help and after every send. Known scarcity
  selects far better than a cap discovered by hitting it, which makes the hard limit a backstop
  rather than the mechanism.
- **The instructions lead with the negatives**, in the brief, the help, and every refusal, because the
  failure mode is over-sending. The one-line test: if you say nothing, does this fact disappear?

## Impact

- **Specs:** `05-workflow` (the channel, its budget, and the register).
- **Code:** `internal/hub/fyi.go` (the constants and the verb), `internal/hub/workflow/prompts_fyi.go`
  (every word the agent reads), `internal/hub/store/workflow.go` (the per-claim grant),
  `internal/hub/store/mail.go` (the fleet window), the two claim paths that grant it, and
  `internal/hub/agent/lifecycle.go` (the reserved name).
- The budget fails CLOSED: an agent that has claimed nothing has nothing granted, so a missing row
  cannot read as an unlimited purse.
- Nothing here reads the user's mailbox — that is the epic's other half, along with the filter and the
  badge that have to know about a recipient who is not an agent.
