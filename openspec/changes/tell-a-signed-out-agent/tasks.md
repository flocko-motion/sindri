# Tasks

## 1. Carry the sender's answer

- [x] 1.1 `/tell` carries an answer about a signed-out pane — refuse, restart, or send — with the
      refusal as the default, so nothing that does not ask changes behaviour.
- [x] 1.2 The hub applies it only where the pane really reads signed out.
- [x] 1.3 `Inject` keeps the guard for hub-originated messages, and `InjectWhenReady` keeps
      recording what never landed.

## 2. Perform the remedy the refusal named

- [x] 2.1 `agent.RestartAgent` replaces the pod on the same identity; `RebuildAgent` uses it in
      place of the stop-then-launch it had inline.
- [x] 2.2 A restart-and-send waits for the session and delivers past the guard, since the pane it
      was restarted for has yet to redraw.
- [x] 2.3 A failed restart reports itself as one.

## 3. Ask in both front-ends

- [x] 3.1 The TUI opens a choice — restart and send, send anyway, cancel — instead of surfacing the
      refusal.
- [x] 3.2 `sindri agent tell` gains `--restart` and `--anyway`, and its refusal names both.

## 4. Pin it

- [x] 4.1 A hub-originated message still refuses a signed-out pane, and names the restart.
- [x] 4.2 Send-anyway reaches the session; restart tears the pod down on the way.
- [x] 4.3 An answer given about an agent that reads fine changes nothing.
- [x] 4.4 The TUI asks for a signed-out agent and only for one.
- [x] 4.5 The CLI refusal names both flags, and each flag maps to its answer.

## 5. Look for the same shape elsewhere

- [x] 5.1 Checked every refusal that reads an observed runtime state: the pane is refused on in one
      place only, and `sindri attach` already tries rather than trusting the board's last word.
