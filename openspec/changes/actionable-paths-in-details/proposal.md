# A path shown in an item's detail is somewhere you can go

## Why

The PRs tab shows the authoring agent's worktree as an actionable item: focus it and ENTER
opens a shell there, `y` copies it. That behaviour comes from one classification on the item
(`kind: "path"`), and it is written down nowhere, so the Agents tab — the tab that is ABOUT
the agent — printed the same tree as plain text. The one place a user goes to ask "where is
this agent working?" was the one place the answer could not be taken anywhere.

The Agents detail also showed the repo-relative form (`.worktrees/dvalin`), which names the
tree without locating it. `agent info` prints the same relative string, so neither front-end
gave a path a user could paste.

## What changes

- An agent's detail shows its workspace as an ABSOLUTE path, and as an actionable item: it
  takes right-column focus, ENTER opens a shell there, `y` copies it.
- The rule is written down for item details generally, so the next detail that names a
  filesystem path is not a third case decided from scratch.
- The path shows whenever it resolves, rather than only while the agent has an open PR. It
  is the agent's workspace: it exists as long as the workspace does, and a field that comes
  and goes with review state reads as a bug.
- When the project root is unknown there is nothing to join the relative workspace to. The
  field still shows, in its relative form, and is not offered as somewhere to open — a shell
  started at a relative path lands wherever the TUI happens to be running.

## Impact

- Specs: `view-tui` gains the requirement; the existing PRs behaviour it describes is
  unchanged and now specified.
- Code: `internal/ui/tui/tab_agents.go` only. No new key and no new action — the existing
  `y`, ENTER and `o` handlers already act on a `path` item.
