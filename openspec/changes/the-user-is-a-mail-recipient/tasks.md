# Tasks

## 0. The sender is stated (sd-bc3a1f)

- [x] 0.1 `Delivery` carries the sender, so a call site states how a message travels and who it is from
      in one argument — no adjacent name strings to transpose.
- [x] 0.2 `senderOf` (the "[user] " sniff) is gone; an unstated sender means the hub.
- [x] 0.3 The senders that are not the hub are named where they are sent: a reviewer's or the user's
      rejection (`who`), a scrapped PR, an amended review.
- [x] 0.4 `fyi` and `MailAgent` go through the one delivery path rather than writing to the store, so
      an agent's name reaches `Mail.Sender` — the third of the four, previously unreachable.
- [x] 0.5 A push to the USER is never attempted: their mailbox is the channel.

## 1. The user as a recipient

- [x] 1.1 `api.Mail.Agent` may name the user, using the same spelling that stamps a message from them.
- [x] 1.2 No agent may be named it, refused at registration, so the two can never share a mailbox.

## 2. The verb

- [x] 2.1 `fyi <message>`, offered to the worker, reviewer and planner — the roles whose work grants a
      budget. The coauthor is excluded on its own grounds: it is in the room with the user already.
- [x] 2.2 The note lands in the user's mailbox, attributed to the agent, and is never pushed (the user
      has no session to type into).

## 3. The budget, four named constants in one file

- [x] 3.1 `maxNoteLen`: refused, never truncated; the refusal says cut it, and forbids splitting.
- [x] 3.2 `NotesPerClaim`: granted whole at each claim, work-based rather than time-based — and at each
      ROLE's claim: claimLeaf/startSubtask for a worker, assignReview for a reviewer, AssignPlan for a
      planner, so no role sees a verb it can never use.
- [x] 3.3 The grant REPLACES — a claim that ends with notes unspent does not bank them.
- [x] 3.4 `fleetNotesPerHour` over a rolling `fleetNoteWindow`, counted over the mailbox itself so
      there is no second tally to drift.
- [x] 3.5 The budget fails CLOSED: no claim, nothing granted.

## 4. Refusals that teach

- [x] 4.1 Each names where the material belongs instead — the PR body, a task comment, an escalation.
- [x] 4.1b The spent refusal is true of a never-granted agent too, and takes its number from the
      constant rather than hardcoding "both".
- [x] 4.2 The fleet ceiling refuses rather than holds, and says so plainly.
- [x] 4.3 Every refusal is logged, since those counts are the only evidence for the numbers.

## 5. Scarcity the agent can see

- [x] 5.1 The remainder is stated after every send, and in the verb's help via the caller's own state.
- [x] 5.2 The register is in the brief, the help and every refusal, negatives first.

## 5b. The read half (sd-36da91)

- [x] 5b.1 The Mail tab's attention marker counts unread mail addressed to the user, fleet-wide,
      counted over the whole mailbox rather than the board's window.
- [x] 5b.2 Only user-directed mail counts — the rest is agent traffic and a marker over it would be
      permanently lit.
- [x] 5b.3 The user's rows are listed whatever repo is in view, and marked as theirs in a list that is
      mostly agent traffic.
- [x] 5b.4 Out-of-scope rows sit under the shared foreign heading (-> api.ForeignAttentionHeading),
      the same one the Agents and PRs tabs use, rather than a second convention.
- [x] 5b.4b The user's own unread comes out of the SAME table pass as the other tallies: the board is
      rebuilt on every notify for every client, and the mailbox grows for the life of the machine.
- [x] 5b.5 Parity: `sindri mail list --mine` narrows to them, the TUI's `w` reaches the same narrowing
      from any state (immediately when nothing is selected), the listing groups by the same rule, and
      the closing line names how many of the fleet's unread are the user's.

## 5c. The active segment (sd-594dc0)

- [x] 5c.1 `MailActive` = unread OR changed inside `api.ActiveWindow`, the shared constant rather than
      a mail-specific one.
- [x] 5c.2 A message's change time is its read time when set, else its send time.
- [x] 5c.3 It leads `MailFilters`, so the TUI cycle and the CLI help both reach it first, and both
      front-ends default to it.
- [x] 5c.4 The unread badge and marker do NOT follow — they say what needs reading.

## 5d. Agent-to-agent mail (sd-2f38d9)

- [x] 5d.1 `mail <agent> <message...>` — one verb, two halves: bare `mail` still reads.
- [x] 5d.2 Bare names resolve across every project (`store.AgentsNamed`); ambiguity is refused naming
      every candidate, and `<repo>/<agent>` is the way through rather than a required form.
- [x] 5d.3 Attribution is qualified (`repo/agent`) where addressing is bare, so a message from
      elsewhere says where it came from and the reply path stays a bare name.
- [x] 5d.4 Mail only — an agent cannot push, so agents cannot interrupt each other.
- [x] 5d.5 The length cap moved to `deliver.go` as `maxMessageLen`, shared by both channels because the
      reason is shared; the per-claim grant and fleet ceiling stay with the note channel, which is what
      bounds the USER's attention.
- [x] 5d.6 Mailing yourself is refused and points at the log.
- [x] 5d.7 The wake that makes this guarantee real is sd-aa3f93's, and it has landed (-> 5f).

## 5e. Replying by id (sd-63a19b)

- [x] 5e.1 `reply <mail-id> <message...>` for agents; the recipient comes from the stored row, so no
      name is typed and no directory is needed.
- [x] 5e.2 `in_reply_to` on the mail row, carried by the delivery (`Delivery.Answering`), so an exchange
      reads as one.
- [x] 5e.3 Only your own mail: an id is not a licence to read another mailbox.
- [x] 5e.4 A reply to the hub is refused, naming escalate and comment as the paths that reach someone.
- [x] 5e.5 A reply to the user does NOT consume the note grant — it answers a message they chose to
      send — while the length cap still applies.
- [x] 5e.6 Both front-ends: `sindri mail reply <id>` and `i` on the Mail tab, both addressing by id.

## 5f. The hub wakes an agent that has mail (sd-aa3f93)

- [x] 5f.1 `NudgeMailWaiting` on the sweep that already looks at every agent, so an agent that stopped
      asking still reads what it was sent.
- [x] 5f.2 The eligibility rule is SHARED with the rated-work nudge (`idleAndReachable`): alive, at an
      empty prompt, holding nothing, not parked — one mechanism with a second reason, not a second path.
- [x] 5f.3 Keyed on the newest unread id, like the stall nudge is keyed on the idle spell: told once per
      thing waiting, and again when something new arrives.
- [x] 5f.4 Supersedes sd-2f38d9's idle-nudge paragraph, whose 5d.7 line was left open for this.

## 6. Verify

- [x] 6.1 A note reaches the user's mailbox, attributed, unpushed, with the remainder stated.
- [x] 6.2 Over-length is refused, stores nothing, and costs no grant.
- [x] 6.3 The grant is spent, then refused with somewhere else to go; it replaces rather than banks.
- [x] 6.4 The fleet ceiling refuses an otherwise-correct note and logs it.
- [x] 6.5 An agent that has claimed nothing is refused, and no agent may be named "user".
- [x] 6.6 The marker counts the user's unread and not the mailbox's, through the board the hub builds.
- [x] 6.7 A note from another repo is listed, grouped as foreign, and marked; agent traffic from
      another repo is not.
- [x] 6.8 Active keeps the unread and the just-read, drops the long-read, and reads its change time
      from the read stamp; the badge still counts unread.
- [x] 6.9 A reviewer is granted by being given a review, and can send; a coauthor is not offered the
      verb at all; the refusal says nothing false to an agent that has never claimed.
- [x] 6.10 The user's share and the mailbox's own tallies move independently, from one pass.
- [x] 6.11 All four senders are recorded, a tagged body does not decide, `From` does not mutate the
      shared classifications, and a real rejection carries its author.
- [x] 6.12 Across repos by bare name with qualified attribution and no push; ambiguity refused naming
      both, with the qualified form delivering; unknown refused; capped but not budgeted; self refused;
      and the read half intact.
- [x] 6.13 Replying needs no name and threads; the hub is refused with somewhere to go; the user's reply
      is uncharged; another agent's mail is refused; and the user can reply from either front-end.
- [x] 6.14 An idle agent with mail is woken; the same message is not nudged twice and new mail is; an
      agent that needs a human, is parked, or holds work is left alone; an empty mailbox wakes nobody.
- [x] 6.15 `make verify` passes.
