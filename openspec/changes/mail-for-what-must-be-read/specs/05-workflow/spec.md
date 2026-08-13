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
  kickoff after launch or a context clear, a chat broadcast, the meeting reminder, and
  a routine rebase onto an advanced reference. Waking is the entire point: a nudge
  fires again next tick, a broadcast is a stream a newcomer catches up on, and a
  completed rebase is re-readable from the tree.

That the high-volume traffic is push-only SHALL be treated as load-bearing rather than
incidental, because the mailbox is never pruned: a sender that marks chatter as mail
does not merely add noise, it adds noise permanently. The question asked of a new
message is therefore both "may this be lost?" and "does this deserve to be kept for
ever?".

The workflow SHALL have no way to send a message without classifying it. Reaching past
the delivery port to inject directly is push-only by omission, which is exactly the
silent loss this requirement exists to prevent.

#### Scenario: A rejection reaches an agent that was away

- **WHEN** a reviewer rejects a PR whose author cannot be typed into
- **THEN** the feedback is waiting for that author when it next reads its mail

#### Scenario: A nudge is not kept

- **WHEN** the hub nudges an agent that has gone quiet
- **THEN** nothing is stored, and the nudge fires again on the next sweep if it is
  still needed

#### Scenario: An edit does not interrupt

- **WHEN** a planner edits the task an agent is working
- **THEN** the notice is mailed and not pushed, so the agent reads it at its next ask
  rather than mid-turn

#### Scenario: A sender cannot omit the choice

- **WHEN** new code in the workflow sends a message to an agent
- **THEN** it states both properties, because no unclassified path exists to call
