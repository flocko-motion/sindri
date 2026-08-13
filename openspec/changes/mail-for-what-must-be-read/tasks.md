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
- [x] 2.7 And a repo-wide guard for the modules the compiler cannot reach (chat, the agent
      lifecycle): every direct injection site is listed with the reason it is push-only, and an
      unlisted one fails the build. `Tell` is push-only DELIBERATELY — synchronous, so its caller
      sees the failure — and says so on itself.

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

## 6b. The user's two actions (sd-ac5476)

- [x] 6b.1 `hub.MailAgent` mails a user's message and never pushes it — not interrupting is the whole
      reason to choose it — and refuses a name with no roster entry, since mail is durable and would
      otherwise hide a typo for ever.
- [x] 6b.2 `POST /agent/mail` beside `POST /tell`, `client.MailAgent` beside `client.Tell`.
- [x] 6b.3 Both front-ends: `sindri agent mail` beside `agent tell`, and `i` beside `t` in the TUI,
      each one-liner naming which interrupts and which waits.
- [x] 6b.4 The signed-out guard stays on the PUSH path only, so mail reaches exactly the agent `tell`
      cannot.

## 6c. Length pressure on the wire files (sd-cd21aa)

- [x] 6c.1 `server.go` split along the seams that grow separately: the request/response plumbing
      (`httpjson.go`) and the streaming endpoints (`streams.go`), leaving the route table and `Serve`.
      Each states a narrower job than "HTTP/JSON over a socket". 672 -> 535.
- [x] 6c.2 `client.go` split at its transport core (`transport.go`), which is the seam that stays
      separate as endpoints accumulate — deliberately NOT the chat/meeting group, which sd-b5d284 was
      asked to lift. 632 -> 586.

