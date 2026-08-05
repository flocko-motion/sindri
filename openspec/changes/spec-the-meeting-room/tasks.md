# Tasks

Mostly documentation of what ships. One behaviour change: membership from the TUI.

## 1. Confirm the spec matches the code

- [ ] 1.1 Walk `internal/hub/chat/service.go` against the new capability — star
      broadcast, the presence lock and its TTL, the single bounded catch-up dropping
      oldest-first, `NewMeeting` keeping the roster, user-only command interpretation,
      system replies reaching the user alone, best-effort delivery — and correct the spec
      wherever the code differs. The code is the authority here; the spec is being
      written to match it.
- [ ] 1.2 Confirm membership genuinely spans projects (`findAgentProject` resolves by
      globally unique name) and that room membership grants an agent nothing outside its
      own project.

## 2. Membership from the TUI

- [ ] 2.1 Add add-member and remove-member actions to the Chat tab, reaching the existing
      `ChatAdd`/`ChatRemove` client methods, so the room stops being CLI-only for
      membership.
- [ ] 2.2 Declare the keys in `internal/ui/tui/keys.go` (the keymap is the single source
      of truth and the footers are generated from it), respecting the case convention:
      these mutate, so they are uppercase.
- [ ] 2.3 Drop the comment in `keys.go` that documents the gap — "membership is curated
      from the CLI" — once it no longer is.
- [ ] 2.4 Extend the keymap tests: no key collides on the Chat tab, and the new bindings
      reach their actions.

## 3. Verify

- [ ] 3.1 `make verify` passes.
- [ ] 3.2 `openspec validate --all` passes.
- [ ] 3.3 Manual: add and remove a member from the TUI and from the CLI; confirm a
      newcomer receives the history as one delivery; confirm the room refuses agents while
      the user is away and opens when they return.
