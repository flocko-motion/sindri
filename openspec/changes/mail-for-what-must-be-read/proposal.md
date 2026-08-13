# Mail for what must be read, push for what must wake

## Why

Everything the hub sends an agent goes into its tmux session, and that is best-effort by
construction. `chat.deliver` says so in the code: "offline agents are skipped and errors only
logged, because a delivery must never fail a broadcast." The reference-move path is the same. So an
agent that is down, restarting, mid-clear or blocked at a prompt misses whatever was sent, with no
record that it was missed.

That is exactly right for a stall nudge and exactly wrong for a rejection with its feedback — and
today they travel the same way. A reviewer's verdict, a cancelled task, an edit to the task an agent
is holding: each is a one-shot consequence that only reaches the agent if the agent happened to be
awake and idle at that moment.

The fix is not a more reliable injection. It is to notice that senders answer two INDEPENDENT
questions, and to make them say both.

## What changes

- `workflow.Delivery` names the two properties — **Mail** (must be read; kept until it is) and
  **Push** (should act now; wakes the agent, may be lost) — and the three combinations that are
  messages at all. There is no TTL, no lifetime field and no default: a message is in the mailbox
  because it must be read, so nothing expires.
- `hub.Deliver` carries one out: it writes the mail FIRST, then attempts the push, and records on
  the mail row whether that push actually landed. A crash between the two loses only the wake.
- **The workflow can no longer send without classifying.** `InjectWhenReady` is gone from
  `workflow.Deps`, so a sender must state both properties; a source guard catches anyone reaching
  past the port to inject directly.
- Every existing sender is classified — verdicts, assignments, merges, cancellations and a rewritten
  reference are mail+push; a task edit under a working agent is mail-only; nudges, prods, kickoffs,
  broadcasts and a routine rebase are push-only. The full list is in the `05-workflow` delta.
- `agent.InjectWhenReady` now ERRORS when nothing was injected. It used to record the skip and return
  nil, so a message nobody could receive read as delivered — which now matters, because a mail row's
  `pushed` is read from that outcome.
- The mailbox is append-only: `sindri mail` (the agent's verb) hands over everything unread, oldest
  first, and MARKS it read. Nothing is deleted, so the record of what an agent was told survives, and
  a crash between delivery and processing leaves the mail waiting.
- `sindri` with no arguments answers with the unread count when mail is waiting, ahead of any other
  directive: mail is one-shot consequence, and a verdict or a cancellation can change what the next
  action should be. Reading first and asking again is the correct order.
- Relevance is settled at PICKUP, not by a clock. The reply to a pick-up sends the reader back to the
  live state (`sindri`, `sindri task`, `sindri prs`) and says the live state wins where the two
  disagree.
- A Mail section in the section model, so both front-ends get the view: the fleet's mail, newest
  first, filtered by unread-or-all and by recipient, each row saying whether it was pushed, with the
  full body on request. Its badge is unread; it claims no attention marker.
- The unread count rides on `AgentView`, so a human can see an agent with a backlog — the one signal
  that an agent has stopped reading.
- `hub-routing` and `agent-runtime` are amended, because both said injection was the only inbound
  path. They were still true while nothing delivered mail, which is why the amendment lands with the
  senders rather than ahead of them.

## Impact

- **Specs:** `hub-routing` (two paths, both declared), `agent-runtime` (mail resumes an idle agent,
  pick-up is explicit, relevance at pickup), `05-workflow` (the classification, sender by sender),
  `view-workers` (the Mail view and the count on the agent).
- **Code:** `internal/hub/store/mail.go` (the mailbox), `internal/hub/deliver.go`,
  `internal/hub/mailverb.go`, `internal/hub/workflow/delivery.go`,
  `internal/hub/workflow/prompts_mail.go`, every workflow sender, `internal/hub/agent/inject.go`,
  `internal/api/mail.go` + `board.go`, `internal/hub/commands/sections.go`, `internal/hub/state.go`,
  `internal/ui/cli/mail.go`, `internal/ui/tui/tab_mail.go`.
- Retention is unbounded and deliberate: it is text, and the classification keeps the high-volume
  traffic off this path entirely. What bounds is the RENDER — the board carries a window with the
  totals beside it, so a view says what it is not showing.
- Deleting an agent leaves its mail in place, orphaned. That record is often most interesting
  precisely when the agent is gone.
