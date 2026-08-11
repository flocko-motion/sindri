# Specify the meeting room

## Why

The meeting room ships and is unspecified. `internal/hub/chat` (a service with
presence, membership, transcript and broadcast), `internal/hub/store/chat.go` (two
tables), the TUI's Chat tab, and five CLI commands — `sindri meeting
add/remove/join/new/log` — have no openspec capability describing them. The only
occurrence of "chat" in the specs is the *exclusion* of an agent's freeform pane output
from the activity log (`hub`, `view-workers`), which is a different thing entirely.

That breaks the architecture rule that "each view and each action SHALL be defined as an
openspec specification, so that all UI variants align to a single definition" — and the
absence shows: the room's own keymap admits the drift in a comment, "membership is
curated from the CLI: `sindri meeting add/remove`", because the TUI has no way to add or
remove a member. An unspecified feature is where interfaces diverge quietly.

The behaviour is also more interesting than a chat log, and worth writing down before it
is changed by someone who has to infer it: the room is locked until the user is present,
a newcomer's catch-up is deliberately a single line, and "new meeting" keeps the roster
while clearing the history.

## What Changes

This is documentation of shipped behaviour, plus one parity fix.

- **A new `meeting` capability** describing the room: the hub as the star centre, the
  user as a required participant, membership as the user's alone, the bounded transcript
  and the one-line catch-up, and `new meeting` clearing history while keeping the roster.
- **Membership becomes reachable from both front-ends.** The TUI Chat tab gains add and
  remove, so the room stops being a CLI-only surface. This is the one behaviour change.

## Capabilities

### Added Capabilities

- `meeting`: the shared room the user and selected agents talk in — its topology,
  membership, presence rule, transcript, catch-up, reset, and the identical surface both
  front-ends present.

## Impact

- **Documents:** `internal/hub/chat/service.go`, `internal/hub/store/chat.go`,
  `internal/ui/cli/chat.go`, `internal/ui/tui/tab_chat.go`, and the room's client
  methods.
- **Changes:** the TUI Chat tab gains membership actions (`ChatAdd`/`ChatRemove` are
  reachable from the CLI only today).
- The room's glyphs and help text are presentation and move to the shared rendering
  module under `separate-hub-from-frontends`; this capability describes *that* they are
  identical in both front-ends, not where they live.
