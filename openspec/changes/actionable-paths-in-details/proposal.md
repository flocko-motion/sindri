# An agent's workspace is somewhere you can go

## Why

The PRs tab shows the authoring agent's worktree as an actionable item: focus it and ENTER
opens a shell there, `y` copies it. That behaviour comes from one classification on the item
(`kind: "path"`), and it is written down nowhere, so the Agents tab — the tab that is ABOUT
the agent — printed the same tree as plain text. The one place a user goes to ask "where is
this agent working?" was the one place the answer could not be taken anywhere.

The Agents detail also showed the repo-relative form (`.worktrees/dvalin`), which names the
tree without locating it.

The gap is the TUI's Agents detail specifically. On the host, `sindri agent dir <name>` prints
the absolute path and documents the `cd "$(sindri agent dir <name>)"` idiom, so a pasteable
path is available from the CLI — behind a separate command from the one that shows the agent's
fields, but available.

## What changes

- An agent's detail shows its workspace as an ABSOLUTE path, and as an actionable item: it
  takes right-column focus, ENTER opens a shell there, `y` copies it.
- The rule is written down for the agent workspace wherever a detail names it, which is the
  Agents detail and the PRs detail, so those two do not stay a matched pair by accident.
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
- The Repos detail also names a path (`tab_repos.go`) and cannot focus, open or yank it: that
  detail is built as plain strings, with no actionable items at all. The requirement here is
  deliberately scoped to the agent workspace rather than claiming a general rule the product
  does not yet obey; the Repos gap is left to its own change.
