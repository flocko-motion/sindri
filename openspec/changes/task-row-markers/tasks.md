# Tasks

## 1. Switch the TUI glyphs to Nerd Font icons (sd-4fea6e)

- [x] 1.1 One marker set in `internal/ui/theme`, holding every symbol both front-ends draw.
- [x] 1.2 The pictorial markers become Private Use Area icons; the ones that read well in any font
      stay plain, and each says why beside it.
- [x] 1.3 The CLI reads the same set, so the eye, the warning and the retired mark cannot drift
      from the dashboard's.
- [x] 1.4 The marker column measures its width from the marks, the hammer having been two cells
      where the wrench is one.
- [x] 1.5 Pinned: every marker is one cell, distinct, and in the PUA; the column fills its width
      for every combination a row can carry.

## 2. Name the agent and mark the pull request (sd-569550)

- [x] 2.1 One rule in `internal/api` for who is behind a task — the holder, the feature container's
      holder, else the author of the open PR — read by the row marker, the detail pane and the CLI,
      so a marked row always has a name behind it.
- [x] 2.2 The detail keeps naming an agent while the PR waits on a verdict, which is when a reader
      most wants it.
- [x] 2.3 `sindri task info` prints the same field, from the same rule.
- [x] 2.4 The name stays out of the row: the marker column is padded so every title starts at the
      same place, and a name is as long as it happens to be.
- [x] 2.5 The PR diamonds become the pull-request pair — the icon and its draft form — so the mark
      reads as a PR and still says which of the two it is.
- [x] 2.6 Pinned: the submitted case, a live claim outranking an old PR, a feature naming its
      holder, a finished PR naming nobody, and the row agreeing with the detail.
