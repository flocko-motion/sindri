## ADDED Requirements

### Requirement: Worker can re-test mergeability on demand

A worker SHALL be able to ask the hub to bring its branch up to its base, as often as it wants, and learn the result. The hub reports one of: already current, rebased cleanly, or conflicted — and when conflicted, which files conflict. This lets a worker iterate toward a mergeable branch instead of discovering the problem only at the human merge.

#### Scenario: Branch is behind but clean

- **WHEN** a worker asks the hub to test mergeability and the branch merely trails base
- **THEN** the hub rebases it onto base and reports success, with no conflict to resolve

#### Scenario: Branch conflicts with base

- **WHEN** a worker asks the hub to test mergeability and the rebase conflicts
- **THEN** the hub reports the conflict and names the files that need resolving

#### Scenario: Worker re-asks after editing

- **WHEN** a worker asks again after editing the conflicted files
- **THEN** the hub resumes the git operation from where it stopped, not from scratch

### Requirement: Hub performs all git; the worker only resolves content

The hub SHALL perform every git operation (rebase, stage, continue, commit) host-side, because the worker has no git access — only its worktree files are mounted. On a conflict the hub SHALL surface the conflict into the worker's worktree (leave the conflict markers in place, not abort), so the worker resolves the file *content*. The worker SHALL never be asked to run git or to perform host/operator setup.

#### Scenario: Conflict is left in the worktree to resolve

- **WHEN** the hub's rebase of a worker's branch conflicts
- **THEN** the conflicted files remain in the worker's worktree with conflict markers, and the rebase is left in progress rather than aborted

#### Scenario: Hub advances the rebase after the worker resolves

- **WHEN** the worker has removed the conflict markers and asks the hub to continue
- **THEN** the hub stages the resolved files and continues the rebase host-side, surfacing the next conflict or reporting completion

#### Scenario: Worker is never told to run git or infra commands

- **WHEN** the hub reports a conflict to a worker
- **THEN** the message describes which files to edit, and never instructs the worker to run git or any host/operator command

### Requirement: A branch reaches the human merge only when it applies cleanly

The hub SHALL NOT hand a branch that conflicts with its base to the human merge. A conflict discovered at merge time SHALL route the branch into the worker-driven resolution loop rather than a dead-end rejection, and once the branch applies cleanly onto base the local PR SHALL be renewed for re-review so the human merge is conflict-free.

#### Scenario: Merge-time conflict enters the resolution loop

- **WHEN** a human triggers a merge and the branch conflicts with base
- **THEN** the branch is routed to its worker for content resolution (the resolution loop), not rejected as "resubmit and try again"

#### Scenario: Clean branch renews the PR for review

- **WHEN** a worker's branch has been rebased cleanly onto base
- **THEN** the local PR is renewed and re-offered for review, after which the human merge applies without conflict
