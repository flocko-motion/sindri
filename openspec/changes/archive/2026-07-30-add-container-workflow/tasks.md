# Tasks

Proposal — not yet implemented. The structured leaf workflow stays as-is; this
adds the container path alongside it.

## 1. Assignment

- [x] 1.1 Derive leaf vs container from the open set (a task is a container if any
      open task has it as `ParentID`); make `claimNext` skip containers.
      (`store.OpenLeaves`; `claimNext` now uses it.)
- [x] 1.2 Recognise a container marked for collaboration (the `collab` label) and
      assign it as a unit to a free agent (`store.MarkedContainers`,
      `Hub.claimContainer`); children reserved via `OpenLeaves`. `claimNext` now
      tries a marked container before the leaf queue.

## 2. Branch + state

- [x] 2.1 Decoupled `store.AgentState.Branch` from `Task` via a new `Container`
      field; for a container `Branch = container.ID` (set once via `EnsureBranch`)
      while `Task` (the current subtask) cycles.
- [ ] 2.2 Render the agent on the board with the container as context and the
      current subtask as the headline. (TUI — pending.)

## 3. Checkpoints

- [x] 3.1 Non-blocking `checkpoint` verb (`Hub.cmdCheckpoint`): commit the current
      subtask to the container branch, close that child, advance to the next open
      child (`advanceContainer`); rests holding the container when the stream is
      empty. Surfaced for container-holders only; `submit` hidden, `next` hidden.

## 4. Milestone PRs

- [x] 4.1 A milestone PR: `Hub.MilestonePR(agent)` opens/refreshes a merge-intent
      on the container branch for its current state and blocks the agent. Wired
      host-side: `client.MilestonePR`, `POST /milestone`, `sindri pr milestone
      <agent>`. (On-request; auto-on-completion deferred — `checkpoint` rests.)
- [x] 4.2 A milestone `Merge` variant: in `Hub.Merge`, a PR whose held owner's
      container == branch lands, fast-forwards the branch past the merge
      (`RebaseOnto`), and resumes the agent (`resumeContainer`) — branch kept,
      agent not freed. `AgentDirective` frees the agent only when the container
      task itself is closed.
- [ ] 4.3 Advisory review: requested review delivers feedback without gating the
      merge. Needs the human approve+merge path (the add-host-pr-approve change) so
      the user can review+merge a milestone without a reviewer agent — NEXT.

## 5. Verify

- [ ] 5.1 `openspec validate --all` and `sindri lint all` pass.
- [ ] 5.2 Bulk: a container with pre-filled children is worked end-to-end on one
      branch and lands as one PR. Interactive: subtasks fed live, milestone PRs on
      request merge without retiring the branch; the agent keeps working across
      merges.
