# Telling a signed-out agent offers the restart that fixes it

## Why

Telling an agent whose pane reads signed out came back as a refusal, and the refusal named a remedy
it did not offer: restart the agent, because the process re-reads the credentials the hub keeps
staged. The right answer was in the modal; the modal could not act on it.

The reasoning behind the refusal is sound for the message it was written for. Text typed at a
`/login` prompt is never sent — it piles up in the input box unread — and a verdict or an assignment
the hub injects has nobody present to notice that. It has to fail loudly.

A user's own message is a different case, twice over. The reading is a PREDICTION: the pane is
looked at, memoised for a couple of seconds, and pattern-matched, so it can be older than what the
user knows — they may have renewed the host token seconds ago. And there is a human right there,
who can be asked. Refusing an action because it will probably fail is not the same as refusing one
that cannot work, which is what the state machine's own gates are for.

## What changes

- A `/tell` carries the sender's answer about a signed-out pane: refuse (the default), restart the
  agent and deliver once it is back, or deliver regardless. The hub applies the answer only where
  the pane actually reads signed out, so one given in advance never bounces a healthy session.
- The restart becomes something the hub does rather than recommends: `agent.RestartAgent` replaces
  the pod on the same identity — the worktree, socket and log belong to the agent — and the message
  goes in once the session is back. `RebuildAgent` uses the same step it had inline.
- The TUI asks instead of refusing: telling a signed-out agent opens a choice — restart and send,
  send anyway, or cancel — with the restart leading the answers and cancel under the cursor.
- `sindri agent tell` gains `--restart` and `--anyway`. Unasked, it still refuses, but the refusal
  names both flags rather than a command the user must reconstruct.
- Hub-originated messages keep the guard exactly as it was: `Inject` refuses a signed-out pane, and
  `InjectWhenReady` records what never landed.

## Impact

- Specs: `agent-runtime` (what a message does about a signed-out pane), `view-tui` (the choice).
- Code: `internal/api/requests.go`, `internal/hub/agent/inject.go`, `internal/hub/agent/lifecycle.go`,
  `internal/hub/server.go`, `internal/client/client.go`, `internal/ui/cli/agent.go`,
  `internal/ui/tui/tell_choice.go`, `internal/ui/tui/component_input.go`.
- `Tell` grows the answer as a parameter, so both front-ends state it and neither can drift into
  guessing what the other does.
- Checked for the same shape elsewhere: the pane reading is refused on in one place only, and
  `sindri attach` already tries whatever the board says rather than refusing on it.
