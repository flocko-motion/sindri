# 04-workers — delta

## ADDED Requirements

### Requirement: Agent creation in `_global` accepts only a reviewer

`NewAgent` SHALL refuse to create any role but reviewer in the virtual project `_global`, since a
worker, planner or coauthor each holds something across its unit of work — a branch, a standing
conversation, the user's own seat — that a project with no repo cannot give it. The refusal SHALL
name what the refused role would have held. A repo keeping its own, project-bound reviewer SHALL
remain unaffected — `_global` is an ordinary project, so the two coexist without a special case.

#### Scenario: A reviewer can be created in `_global`

- **WHEN** a reviewer is registered in `_global`
- **THEN** it is accepted, exactly as in any other project

#### Scenario: Every other role is refused

- **WHEN** a worker, planner, or coauthor is registered in `_global`
- **THEN** it is refused, and the refusal names what that role holds that `_global` cannot

### Requirement: A `_global` reviewer's pod holds no repository

A pod launched for a `_global` reviewer SHALL NOT check out or check for a git repository: there is
none to check out. Launch SHALL skip the commit-check and worktree-add steps that every other role's
pod requires, creating only the fixed workspace directory its bind mount needs. The pod's mounts
SHALL be no different from a project-bound reviewer's — `/workspace` alone, no worktree or scratch
mount — so no mount-table change is needed for the role itself.

#### Scenario: A `_global` reviewer launches without a repository

- **WHEN** a reviewer's pod is launched in `_global`
- **THEN** it succeeds without any git operation against `_global`'s path, and its workspace
  directory exists once launched

### Requirement: A global reviewer's workspace is materialised per review

Assigning a review to a `_global` reviewer SHALL populate its fixed workspace directory with the
PR's tree as plain files — no `.git`, since the reviewer has no git of its own and the hub runs the
curated git surface and the lint gate hub-side. The materialise SHALL replace whatever the previous
review left there, without restarting the pod (the workspace is a bind mount; replacing its contents
is the whole change). A failed materialise SHALL be reported the same way a failed checkout already
is for a project-bound reviewer: the reviewer is told `/workspace` does not hold the PR and to read
the diff alone.

#### Scenario: A review's tree replaces the last one

- **GIVEN** a `_global` reviewer's workspace holding files from a prior review
- **WHEN** it is assigned a new review
- **THEN** the new PR's tree is there and the prior review's files are gone, with no `.git` present

#### Scenario: A failed materialise still hands over the review

- **WHEN** materialising a `_global` reviewer's workspace fails
- **THEN** the reviewer is still assigned the review and told not to trust `/workspace`, the same
  guarantee a failed checkout already gives a project-bound reviewer

### Requirement: A reviewer's context is cleared when its verdict lands

Recording a reviewer's verdict SHALL clear its session (Claude Code's own `/clear`), not compact it,
and SHALL do so at the verdict rather than at its next assignment. A summary would keep the
conclusions of the review just finished while discarding the diff that justified them, which is
backwards for a role whose work is reading that diff; and clearing at the verdict — a leaf boundary
by construction — means the fresh window is ready before the next review arrives rather than costing
latency when it does. The clear SHALL re-serve the reviewer's directive itself once it settles, so a
cleared reviewer is never left waiting to be told what to do. A reviewer's session SHALL carry
nothing from one review into the next — no conclusion, no judgement — which matters most for a
`_global` reviewer moving between unrelated repos.

#### Scenario: A verdict clears the reviewer

- **WHEN** a reviewer approves or rejects a PR
- **THEN** its session is cleared, not compacted, and it is left able to ask for its next review
  without further prompting
