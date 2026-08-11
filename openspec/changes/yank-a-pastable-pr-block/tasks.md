# Tasks

## 1. One definition of what identifies a PR

- [x] 1.1 `prIdentity` builds the identifying lines, including the author's worktree path.
- [x] 1.2 `prDetailLines` builds on it, so the ENTER modal keeps everything it had — feedback,
      reviews, history and diff — and gains the path with no second place to maintain.

## 2. Yank the block from the list

- [x] 2.1 `y` on the PRs list copies the block; the detail pane keeps yanking the focused value.
- [x] 2.2 Fall back to the id while the lazily-fetched detail is still another PR's, so the
      clipboard never holds one PR's fields under another's name.

## 3. Pin it

- [x] 3.1 The block carries the id, status, author, task id and title, and the path, and not the
      diff.
- [x] 3.2 The detail pane's yank is unchanged.
- [x] 3.3 The stale-detail fallback.
- [x] 3.4 A PR whose author is gone keeps its block and drops only the path.
- [x] 3.5 A drift guard: every line of the block appears in the full detail, and the full detail
      still carries the diff.
