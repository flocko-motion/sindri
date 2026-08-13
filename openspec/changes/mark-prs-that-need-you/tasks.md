# Tasks

## 1. State the rule

- [x] 1.1 `api.PRWaitReason`: merge-failed, approved, open and interim, or open with no reviewer
      alive; rejected waits on its author and merging on the merge. `PRNeedsUser` reads it as a bool.
- [x] 1.2 `api.AnyLiveReviewer` judges liveness in the PR's own repo, over `AgentNotUp`'s word list.
- [x] 1.3 Say why it is per repo (assignment is), and why a stuck reviewer is the Agents marker's.

## 2. One more line in the registry

- [x] 2.1 The PRs section gains its attention recipe; `BoardState` gains the count it reads.
- [x] 2.2 Nothing in the TUI: the handles have been drawn from the resolved sections since the
      groundwork landed, so this marker arrives with no view change.

## 3. The CLI renders the reason, it does not re-derive it

- [x] 3.1 Each waiting row says which state it is in, from the value the rule returned.
- [x] 3.2 A closing line names the PRs, grouped over `api.PRWaits` by the command that clears each.
- [x] 3.3 A reason with no words here is a test failure, never a confident wrong remedy.

## 4. Pin it

- [x] 4.1 All four waiting states, and every state that is not one: rejected (interim or not),
      merging, merged, scrapped, and an open PR a running reviewer will reach.
- [x] 4.2 Liveness: every word that means a pod is up, every word that means it is not, and a
      roster with no reviewer on it at all.
- [x] 4.3 Two projects, since every other case shares one and so cannot see the scoping: a
      reviewer in repo A leaves repo B's PR waiting on the user.
- [x] 4.4 All three sections' attention counts against one real board, whose live reviewer sits in
      another repo than its PRs.
- [x] 4.5 The PRs handle renders its marker from the section model, alongside the other two.
