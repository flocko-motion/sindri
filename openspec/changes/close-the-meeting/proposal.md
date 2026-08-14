# Meeting room hygiene: close it, and keep it quiet while locked

## Why

The room locks but is never cleaned up. `presenceTTL` is 20 seconds, so it locks almost as soon as
the user stops heartbeating — and nothing ever CLOSES it. Membership is durable and dropped only by
an explicit removal or by deleting the agent, so a room nobody has opened for days still has members,
and those members are still spoken to. Two agents sat in one for days.

## What changes

- **A meeting can be closed**: `C` on the Chat tab (confirmed, since re-adding members is manual)
  and `sindri meeting close` (immediate, as the CLI's other destructive verbs are). Every member is
  removed and told; the transcript is kept, because reading a finished meeting is a different
  question from clearing it, and `meeting new` already owns that.
- **A room with nothing said in it for an hour closes itself**, on the hub's slow loop.
- **Measured from the last message, not from the presence lock.** The transcript is the record of
  the meeting, and an interface left open on the Chat tab heartbeats indefinitely — which would keep
  an empty room alive in exactly the case this exists to end. The trade is that a user reading
  without typing for an hour is treated as having finished; that is the honest reading of a room
  with nothing in it.
- **The automatic close wakes nobody.** A deliberate close may interrupt — the user has just asked
  for it, and an agent that thinks it is still in the room will try to speak into it. An automatic
  one delivers when each agent is next idle, through a second method on the chat Delivery port.
- **It leaves a trace**, since a roster that silently vanished looks like a bug: the reason goes into
  the transcript, and each agent's own log records the notice as membership changes already do.

## Impact

- Specs: `meeting` gains the close, the idle rule and the quiet-delivery constraint.
- Code: `internal/hub/chat/service.go` (`Close`, `CloseIfIdle`, the quiet delivery path),
  `internal/hub/wiring.go` and `internal/hub/refwatch.go` (the tick), `internal/hub/server.go`,
  `internal/client`, `internal/ui/cli/chat.go`, `internal/ui/tui` (`onkey.go`, `keys.go`,
  `tab_chat.go`).
- `chat.Delivery` gains `InjectWhenReady`, which the hub already had.
- The meeting's client surface moved to `internal/client/chat.go`: `client.go` crossed the 700-line
  limit once this branch was combined with what landed on it while the PR was open. Split rather
  than shaved, per the linter's own instruction — the next method on `client.go` would put a shaved
  file straight back over. The methods are unchanged, and the block was already contiguous.
- The second subtask of this feature (sd-8a0ae9) lands on the same branch and extends this document.
