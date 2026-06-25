# gh-local — delta

## MODIFIED Requirements

### Requirement: Merge into base, on approval only

Merging a PR SHALL first bring its branch up to the current base by rebasing the
branch onto base, then fast-forward-free merge the branch into the base branch and
mark the PR merged. A PR SHALL only be merged after it is approved, and merging
SHALL be gated by the task's review gates. When the rebase hits conflicts — a
genuine divergence from base — the merge SHALL NOT proceed: the PR SHALL be routed
back to the owning worker to resolve and resubmit, and the conflict SHALL be
reported rather than silently swallowed.

#### Scenario: Gated merge

- **WHEN** a merge is attempted while the task has an unmet review gate
- **THEN** the merge is refused until the gate is satisfied

#### Scenario: Stale branch merges after auto-rebase

- **WHEN** an approved PR whose branch has fallen behind the base is merged
- **THEN** the hub rebases the branch onto the current base and, the rebase being
  clean, completes the merge with no human step

#### Scenario: Conflict returns to the worker

- **WHEN** rebasing the branch onto the current base conflicts
- **THEN** the merge stops, the PR returns to its owning worker with the conflict
  reported, and the worker resolves and resubmits
