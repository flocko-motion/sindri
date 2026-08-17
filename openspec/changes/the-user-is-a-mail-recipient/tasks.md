# Tasks

## 1. The user as a recipient

- [x] 1.1 `api.Mail.Agent` may name the user, using the same spelling that stamps a message from them.
- [x] 1.2 No agent may be named it, refused at registration, so the two can never share a mailbox.

## 2. The verb

- [x] 2.1 `fyi <message>`, open to every role — any agent can notice something in passing.
- [x] 2.2 The note lands in the user's mailbox, attributed to the agent, and is never pushed (the user
      has no session to type into).

## 3. The budget, four named constants in one file

- [x] 3.1 `maxNoteLen`: refused, never truncated; the refusal says cut it, and forbids splitting.
- [x] 3.2 `NotesPerClaim`: granted whole at each claim, work-based rather than time-based.
- [x] 3.3 The grant REPLACES — a claim that ends with notes unspent does not bank them.
- [x] 3.4 `fleetNotesPerHour` over a rolling `fleetNoteWindow`, counted over the mailbox itself so
      there is no second tally to drift.
- [x] 3.5 The budget fails CLOSED: no claim, nothing granted.

## 4. Refusals that teach

- [x] 4.1 Each names where the material belongs instead — the PR body, a task comment, an escalation.
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
- [x] 5b.5 Parity: `sindri mail list --mine` narrows to them, the listing groups by the same rule, and
      the closing line names how many of the fleet's unread are the user's.

## 5c. The active segment (sd-594dc0)

- [x] 5c.1 `MailActive` = unread OR changed inside `api.ActiveWindow`, the shared constant rather than
      a mail-specific one.
- [x] 5c.2 A message's change time is its read time when set, else its send time.
- [x] 5c.3 It leads `MailFilters`, so the TUI cycle and the CLI help both reach it first, and both
      front-ends default to it.
- [x] 5c.4 The unread badge and marker do NOT follow — they say what needs reading.

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
- [x] 6.9 `make verify` passes.
