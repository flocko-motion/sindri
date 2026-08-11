# An agent can read the comments on its own task

## Why

The hub attaches a task's comment thread to every `TaskInfo`, so the data reaches the reader.
The agent-facing task view then dropped it: an agent could be commented at, by a human or by
the GitHub issue upstream, and had no way to read the message. The agent that reported this
went looking through its PR and the files for a report it could not find, because the surface
it was written on was the one surface that did not render it.

This is about to get worse. `sd-1c3041` gives agents a comment verb. An agent able to write a
comment that neither its own task view nor anything else it can reach displays would file a
finding and be unable to read it back, which is worse than having no comment verb at all.

## What changes

- The agent-facing task view renders the thread, in both places it shows a task: bare `task`
  (the view an agent reaches for first) and `task <id>`.
- Bare `task` reads the task row, and the thread lives in its own table, so this is a fetch as
  well as a render. `task <id>` already had the comments attached and only lacked the render.
- Every surface names the comment's SOURCE alongside author, time and body. The source says who
  else has already seen it: `github` means it came from or went to the upstream issue, `td` marks
  a thread synced before that import. The TUI was showing author and time only, so it gains the
  source; the host CLI's `task info` already printed all four.
- An empty thread renders nothing at all — no heading, no count. These views are re-read
  constantly, and a heading that is always present for an absent thing teaches skimming.

## Impact

- Specs: `05-workflow` gains the requirement, next to the existing communication-via-comments one.
- Code: `internal/hub/workflow` (the agent-facing view and a shared renderer) and
  `internal/ui/tui/tab_tasks.go` (the source on the head line).
- The CLI needed no change; it already rendered the thread with every field.
- Scoped to a task's OWN detail view. The TUI's peek modal and the PRs tab's linked-task modal
  render a task from the board snapshot, which carries no comments — those are lazily fetched for
  the selected task alone — so they show a task without its thread and the requirement does not
  claim otherwise.
