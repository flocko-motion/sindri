# Tasks

## 1. Let a PR that needs you cross repos

- [x] 1.1 `prVisible` mirrors `agentVisible`, calling `api.PRNeedsUser` rather than restating it.
- [x] 1.2 The PRs tab lists through it, so an approved PR elsewhere is no longer invisible.
- [x] 1.3 The tab badge counts through the same predicate — badge and list are one claim rendered
      twice, and were allowed to disagree.

## 2. Make a foreign row legible

- [x] 2.1 `api.SortedPRs` groups by repo path, stable so the store's newest-first order survives
      inside each repo. One sort, called by the PRs tab and `sindri pr list` alike.
- [x] 2.2 The scope label reads `repo+needs-you`. It was `repo`, which denied the exception the
      Agents tab has had all along.

## 3. Parity

- [x] 3.1 `sindri pr list` sorts the same way and prints the repo column `agent list` already has —
      it is a fleet-wide listing, so a row without its repo cannot be placed.
- [x] 3.2 `api.RepoName` is the single tag→name lookup; the TUI's private copy now calls it.

## 4. Leave the Tasks tab alone

- [x] 4.1 Not touched, and the spec delta says why: a proposal awaiting a verdict is foreground
      work, seen because the user is already in that repo.

## 5. Pin it

- [x] 5.1 An approved foreign PR shows in the narrow scope; the active repo's own still show; a
      foreign PR with a reviewer of its own does not — without that last case the toggle would be
      doing no work at all. Mutation-checked against the bare `inScope`.
- [x] 5.2 Foreign rows group by repo rather than interleaving by age.
- [x] 5.3 The label does not claim to exclude what it shows.
- [x] 5.4 The badge equals the rows rendered, in both scopes.
