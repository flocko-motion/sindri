# A child added mid-work extends the PR, rather than stranding the worker or merging over it

## Why

Adding a child to a task an agent is already working should RAISE THE BAR for its pull request: the
agent takes the new work on too, and the whole hierarchy merges together. Today it does neither
consistently, and the two halves fail in opposite directions.

A leaf worker submits regardless. `CmdSubmit` only asks about children when the worker holds a
feature (`if st.Container != ""`), so a task that gained one goes up as though it had not — and the
merge then closes it over that open child. That is the incident the checkpoint guard was written
for: an epic handed out as a subtask was closed over four open children of its own, and nothing
downstream could tell.

A worker mid-feature is stranded instead. `CmdCheckpoint` refused when the current subtask had open
children, and the agent cannot close the child, re-parent it, or approve anything. Its only exit was
a human noticing.

## What changes

- A task that gains a child while an agent is working it becomes a FEATURE held by that same agent.
  The machinery already exists — one agent, one standing branch, subtasks checkpointed one after
  another, one PR at the end — and the branch name already matches, because a leaf branch is named
  for its task. The state moves from `{Task: X}` to `{Container: X, Branch: X}`.
- The promotion assigns nothing itself. The agent is parked on the feature and the ordinary
  directive picks the subtask, which is the only place that asks every question about what may be
  handed out — and asks them later, after the approval row a proposal gets a moment after the task.
  Assigning at promotion time read the gate before it existed and handed out unreleased work.
- The agent is told: what was added, and that its PR now covers both. Its unit changed shape by
  someone else's act; it must not discover that by being refused at a checkpoint.
- `CmdCheckpoint` no longer refuses a subtask that has open children. It records the work, leaves
  that task OPEN, and hands over the next open leaf — `closeCompletedAncestors` closes it when its
  children are done. The invariant it protected is unchanged: the parent still never closes over its
  children. Only the dead end is gone.
- `CmdSubmit` on a leaf whose task has open children extends rather than refuses: it promotes, says
  what it holds instead of a PR, and points at the subtask. Reached when the child arrived while the
  agent was not running, so nothing promoted it at the time.
- The merge holds the same line for every shape of PR: a task with open children is landed as a
  MILESTONE — branch in, task still open, worker still on it — rather than closed. A PR already out
  when the child arrives is deliberately not moved at that moment; this is the guard that catches it.
- `store.OpenChildIDs` counts every unfinished status rather than the literal `open`. A child being
  WORKED is the plainest case of "closing this parent would be a lie", and asking only for `open`
  hid it from every caller: close, reconcile, checkpoint and both new guards.

## The two edges, decided

**Nesting.** One level down needs no promotion. A subtask that gains a child is still inside the
same feature on the same branch, and the feature loop already reaches it: `OpenSubtasks` serves
leaves at ANY depth and `closeCompletedAncestors` finishes the intermediate parents. So the agent
keeps the state it has, is told, and the checkpoint change is what removes the strand. Nothing new
is built for nesting, and nothing is refused for it either.

**A child that was not meant to block.** Every open child blocks, and that is the default because
silent non-blocking is how a parent gets closed over real work. The way to un-block is the verdict
the user already has: REJECT the child, which blocks neither claiming nor completion, is on record
with a reason, and is reversible. Work genuinely meant for later belongs somewhere else in the tree,
not parked under something in flight. No new state, no new field, and no way to park a child
silently.

## Impact

- Specs: `hub` gains the requirement for a task that grows under its worker.
- Code: `internal/hub/workflow/adopt.go` (new — the promotion and the notice), `feature.go` (the
  checkpoint carries on), `pr.go` (the submit extends), `merge.go` (the invariant), `task.go` (both
  paths that write a parent link), `prompts.go`, `injected.go`, `internal/hub/store/tasks.go`.
