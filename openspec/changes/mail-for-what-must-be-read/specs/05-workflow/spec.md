# Workflow — delta

## ADDED Requirements

### Requirement: Every sender declares whether a message must be read

Every message the hub sends an agent SHALL be classified by its sender, which answers
both questions at the point of sending: must the agent READ this, and should it ACT
NOW. There SHALL be no default and no lifetime to get wrong — a sender that states
neither is a fault, not a quiet no-op.

The classification of the senders that exist today SHALL be:

- **mail and push** — a reviewer's verdict, a rejection with its feedback, a review
  assignment or amendment, a plan brief, a merge or a milestone merge, a contribution
  merged, a conflict the author must resolve, a cancelled task, a scrapped PR, a
  cancelled review, and a reference branch that was REWRITTEN under an agent. Each
  must not be missed, and each should be acted on now.
- **mail only** — a task edited under the agent working it, and hierarchy changes to
  the unit it holds. It must be read, and it must not interrupt a working agent
  mid-turn.
- **push only** — a stall nudge, a cut-off-turn retry, a work-available prod, a
  kickoff after launch or a context clear, a chat broadcast, the meeting reminder, a
  routine rebase onto an advanced reference, and a user's `tell`. Waking is the entire
  point: a nudge fires again next tick, a broadcast is a stream a newcomer catches up
  on, and a completed rebase is re-readable from the tree. A `tell` belongs here for a
  different reason — it is SYNCHRONOUS and its caller is a person, so a failure is
  reported to the terminal that typed it rather than lost, and what a user says while
  watching is conversational steering that must not accumulate for ever.

That the high-volume traffic is push-only SHALL be treated as load-bearing rather than
incidental, because the mailbox is never pruned: a sender that marks chatter as mail
does not merely add noise, it adds noise permanently. The question asked of a new
message is therefore both "may this be lost?" and "does this deserve to be kept for
ever?".

The workflow SHALL have no way to send a message without classifying it. Reaching past
the delivery port to inject directly is push-only by omission, which is exactly the
silent loss this requirement exists to prevent.

Where a module outside the workflow holds its own handle on the session — the chat
relay, the agent lifecycle — its injection sites SHALL be enumerated with the reason
each is push-only, and an unlisted one SHALL fail the build. The compiler cannot reach
those, and an audit that merely looked found one and missed another.

