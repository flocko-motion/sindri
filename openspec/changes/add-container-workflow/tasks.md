# Tasks

Proposal — not yet implemented. The structured leaf workflow stays as-is; this
adds the container path alongside it.

## 1. Assignment

- [ ] 1.1 Derive leaf vs container from the open set (a task is a container if any
      open task has it as `ParentID`); make `claimNext` skip containers.
- [ ] 1.2 Recognise a container marked for collaboration (a td label) and assign
      it as a unit to a free agent; reserve its open children to that agent so the
      leaf assigner doesn't hand them out elsewhere.

## 2. Branch + state

- [ ] 2.1 Decouple `store.AgentState.Branch` from the current task; add a
      current-subtask field. For a container, `Branch = container.ID` (set once,
      via the planner's `EnsureBranch` standing-branch pattern) while the current
      subtask cycles.
- [ ] 2.2 Render the agent on the board with the container as context and the
      current subtask as the headline.

## 3. Checkpoints

- [ ] 3.1 Add a non-blocking `checkpoint` (a.k.a. `done`) verb: commit the current
      subtask to the container branch, close that child in td, advance to the next
      open child; the agent stays working (idles only when the stream is empty).

## 4. Milestone PRs

- [ ] 4.1 A milestone PR: open/refresh a merge-intent on the container branch for
      its current state, on request or when children are exhausted.
- [ ] 4.2 A milestone `Merge` variant: block the agent on the milestone PR, land
      current state, rebase the branch onto the new base, then resume the agent on
      the same container (do NOT free/retire). Retire the branch and free the
      agent only when the container closes.
- [ ] 4.3 Advisory review: a requested review delivers feedback without gating the
      merge; human approve+merge stays the path to base.

## 5. Verify

- [ ] 5.1 `openspec validate --all` and `sindri lint all` pass.
- [ ] 5.2 Bulk: a container with pre-filled children is worked end-to-end on one
      branch and lands as one PR. Interactive: subtasks fed live, milestone PRs on
      request merge without retiring the branch; the agent keeps working across
      merges.
