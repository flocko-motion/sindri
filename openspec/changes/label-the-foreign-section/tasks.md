# Tasks

## 1. Say the words once

- [x] 1.1 `api.ForeignAttentionHeading(n)`, `api.LocalHeading` and `api.OtherReposHeading` are the
      wording, in the package both front-ends already share. Two renderings of one heading would
      drift the first time either was reworded.

## 2. Label the sections in the TUI

- [x] 2.1 `sectioned` puts the foreign rows under their heading, above a blank line and the local
      heading, and returns a purely local list untouched.
- [x] 2.2 The Agents tab splits its rows into the two groups, asking `agentVisible` which side a row
      is on rather than restating the rule; `agentRow` renders one row for either group.
- [x] 2.3 The PRs tab does the same through `prVisible`, with `prRow` for the row itself.

## 3. Keep the cursor on items

- [x] 3.1 A heading and its spacer are rows that select nothing — `row.selectable()` names the
      convention `row` already documented.
- [x] 3.2 `moveCursor` steps the selection over them, and every key that moves it — `j`, `k`, `g`,
      `G`, `ctrl+d`, `ctrl+u` — goes through it.
- [x] 3.3 `reclamp` snaps as well as clamps, for the first frame and for a poll that grows a heading
      above the cursor.

## 4. Parity

- [x] 4.1 `sindri agent list` and `sindri pr list` group through one helper: the waiting rows under
      the same heading, then the current repo, then the rest of the fleet under a heading of its own
      rather than under a `Local:` that would be false of it.
- [x] 4.2 Both listings stay flat, in the sort's order, when nothing waits elsewhere — and when the
      command is run outside any registered repo, where nothing is foreign.

## 5. Pin it

- [x] 5.1 Both tabs: foreign rows under a counted heading, above a labelled local section, blank line
      between. Mutation-checked by removing the heading.
- [x] 5.2 No heading and no spacer when every row is local, and none in global scope, where no row is
      admitted by the exception.
- [x] 5.3 The cursor cannot land on a heading, by any key that moves it, from either end — and the
      first frame opens on an item. Both checks fail without the snap.
- [x] 5.4 The heading's count is the rows beneath it, and never more than the fleet-wide attention
      count the badge shows.
- [x] 5.5 The CLI's grouping, its flat ordinary listing, its behaviour outside a repo, and an empty
      section printing no heading.
- [x] 5.6 The three badge tests count the rows a cursor can rest on, so the invariant survives a list
      that also carries labels.
