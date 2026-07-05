# gh-local — delta

## MODIFIED Requirements

### Requirement: Self-contained, no remote dependency

The PR/worktree/merge workflow SHALL function with no network and no GitHub
account; branches, PRs, review, and the human merge gate all live in the local
git repository and never contact a remote. This offline guarantee covers the
*core loop*. It does NOT preclude sindri from having *separate, optional* network
integrations layered beside it — specifically, the GitHub *issue source* (see the
`github-issues` capability), which reads open issues inbound and closes an issue
on merge. Such an integration SHALL be optional and SHALL degrade to absent when
the network or GitHub is unavailable, so its absence never breaks the offline
core: with no network, create, review, and merge of local PRs all still work, and
merge SHALL never block on a GitHub write-back.

#### Scenario: Offline

- **WHEN** sindri runs with no network
- **THEN** create, review, and merge of local PRs all still work

#### Scenario: Merge does not block on GitHub

- **WHEN** a `gh-*` task's local PR is merged while GitHub is unreachable
- **THEN** the merge completes locally and the GitHub close/comment is skipped with
  a warning — the local merge is never blocked on the remote

#### Scenario: Issue source absent, core unaffected

- **WHEN** the GitHub issue source is disabled or unavailable
- **THEN** the local PR/worktree/merge workflow is unchanged and fully functional
