# Tasks

## 1. The mailbox (sd-b342a9)

- [x] 1.1 An append-only `mail` table: no delete, no expiry; reading sets `read_at`.
- [x] 1.2 A `pushed` column, set from the delivery's OUTCOME, so "possibly acted on" and "nothing
      reached it" stay distinguishable.
- [x] 1.3 Fleet-wide reads for the view, per-agent reads for the verb, and tallies over the whole
      mailbox rather than the window.

## 2. The two properties, declared by every sender

- [x] 2.1 `workflow.Delivery` with Mail and Push, and the three combinations named.
- [x] 2.2 `hub.Deliver`: mail first, then the push, then record whether the push landed. A push
      failure is not returned once the mail is written — the message is not lost.
- [x] 2.3 `InjectWhenReady` removed from `workflow.Deps`, so no unclassified path exists to call.
- [x] 2.4 Every sender classified (the list is in the `05-workflow` delta).
- [x] 2.5 `agent.InjectWhenReady` errors when nothing was injected; it used to return nil after
      logging the skip, which read as delivered.
- [x] 2.6 A source guard: nothing in the workflow reaches past the port to inject.

## 3. Pick-up

- [x] 3.1 `sindri mail` hands over everything unread, oldest first, and marks it read.
- [x] 3.2 The reply settles relevance against the live state, since nothing expires.
- [x] 3.3 An empty mailbox says what the mailbox is, not just "nothing".

## 4. Told on `sindri`

- [x] 4.1 The directive answers with the unread count when mail waits, ahead of every role's own
      directive and before the paths that block waiting for work.
- [x] 4.2 It REPLACES rather than accompanies that directive: mail can change what the next action is,
      so reading first is the correct order.

## 5. Visibility

- [x] 5.1 A Mail section in the section model; both front-ends pick up the tab.
- [x] 5.2 Rows: recipient, repo, sender, time, read state, pushed, an opening of the body.
- [x] 5.3 The full body on request (`GET /mail?id=`), in the TUI detail and `sindri mail show`.
- [x] 5.4 Filters by unread-or-all and by recipient, from one shared definition.
- [x] 5.5 The window is disclosed — "showing the last N of M" — in both front-ends.
- [x] 5.6 The unread count on `AgentView`, shown in both front-ends.

## 6. Specs

- [x] 6.1 `hub-routing`: two paths, both declared by the sender.
- [x] 6.2 `agent-runtime`: mail resumes an idle agent, pick-up is explicit, relevance at pickup.
- [x] 6.3 `05-workflow`: the classification, sender by sender.
- [x] 6.4 `view-workers`: the Mail view and the agent's unread count.

## 7. Verify

- [x] 7.1 A message that must be read survives an agent that cannot be reached, and records that no
      push landed.
- [x] 7.2 Push-only stores nothing — the argument that makes unbounded retention affordable.
- [x] 7.3 Two real senders end to end: a rejection is mail+push, a stall nudge is push-only.
- [x] 7.4 The directive reminds, reading clears it, and the record survives marked read.
- [x] 7.5 The window keeps the recent end while the tallies count everything.
- [x] 7.6 Both front-ends: rows, disclosure, filters, the body, the agent's count.
- [x] 7.7 `make verify` passes.
