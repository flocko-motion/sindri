# 04-workers — delta

## ADDED Requirements

### Requirement: A reviewer may belong to the global pool

The hub SHALL accept a reviewer created in `_global`, where it serves every project's review queue
rather than one repo's. The role is eligible because a reviewer holds nothing across reviews: its
assignment lives in the review row, its verdict is written when given, and both its brief and its
tree are replaced at the next assignment.

The reviewer role SHALL be the only one accepted there. Workers, planners and coauthors remain bound
to a repo: each holds something that outlives a single unit of work — a branch, a standing
conversation, the user's own seat — so the reasoning above does not reach them.

A repo MAY still have its own reviewer. `_global` being an ordinary project means the two coexist
without a special case, and a repo with review needs of its own keeps a dedicated one.

#### Scenario: A reviewer is created in the pool

- **WHEN** a reviewer is created in `_global`
- **THEN** it is accepted, and it is served reviews from every project

#### Scenario: Another role is refused

- **WHEN** a worker, planner or coauthor is created in `_global`
- **THEN** it is refused, and the reason names what that role holds across a unit of work

#### Scenario: A repo keeps its own reviewer

- **WHEN** a repo has a reviewer of its own and the pool also has one
- **THEN** both exist, and the repo's own is preferred for its reviews

### Requirement: A global reviewer's workspace is materialised per review

A global reviewer's `/workspace` SHALL be a fixed host path the hub fills, not a git worktree inside a
repo. Assigning a review SHALL materialise the PR's tree there, replacing what the previous review
left.

A plain tree suffices: an agent has no git of its own — the hub runs a curated read-only subset on its
behalf — and the quality gate executes hub-side. A reviewer needs the files, not a repository.

Because the mount is a fixed path, the pod SHALL NOT be restarted to serve a different repo. A bind
mount shows what is at its path, so replacing the contents is the whole of the change.

Where materialising fails, the reviewer SHALL be told plainly that `/workspace` does not hold the PR
and to review from the diff alone — the guarantee the existing checkout failure already makes, and it
must survive the change of mechanism.

#### Scenario: A second review replaces the first tree

- **WHEN** a global reviewer is assigned a review after finishing one in a different repo
- **THEN** its `/workspace` holds the new PR's tree, nothing of the previous one remains, and its pod
  was not restarted

#### Scenario: Materialising fails

- **WHEN** the tree cannot be materialised
- **THEN** the reviewer is told `/workspace` does not hold the PR and to review from the diff alone

### Requirement: A reviewer's context is cleared when its verdict lands

A reviewer's session SHALL be cleared after it records a verdict, not compacted, and not at its next
assignment.

Clearing rather than compacting: a summary keeps the conclusions of the last review and discards the
diff that justified them, which is backwards for a role whose work is reading this diff carefully. It
also carries judgements between unrelated PRs, and for a global reviewer between unrelated repos.

At the verdict rather than the next assignment: that is a leaf boundary by definition, the fresh
window is ready before work arrives rather than costing latency when it does, and it keeps context
operations off the assignment path entirely.

Clearing SHALL re-serve the reviewer's directive as an armed clear already does, so a cleared reviewer
is never left waiting to be told what to do.

#### Scenario: The verdict clears the session

- **WHEN** a reviewer records a verdict
- **THEN** its session is cleared, and its directive is re-served so it is not left waiting

#### Scenario: Nothing carries into the next review

- **WHEN** that reviewer is assigned its next review
- **THEN** its session carries nothing of the previous one
