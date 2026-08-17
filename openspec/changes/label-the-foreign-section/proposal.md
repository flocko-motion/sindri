# Label the foreign section, so a row from elsewhere reads as one

## Why

Repo scope keeps background work that waits on the user, wherever it lives, and the rows are already
gathered by repo. They are still misread as local — several times in one day — because nothing says
what the group IS. A foreign row looks like one of this repo's with an unfamiliar tag in the first
column, and that column is exactly what an eye skims past while scanning for a name or a status.

Grouping answered "are these rows adjacent"; the user was asking "which of these are mine and which
are somebody else's". A heading answers that question, and nothing else on the row can.

## What changes

- Foreign rows sit under `Needing attention in other repos (N):`, above a `Local:` section, with a
  blank line between them. Foreign FIRST: needing the user is the only reason those rows are on
  screen at all, and putting them beneath the local list would recreate the discoverability problem
  that admitting them fixed.
- The heading carries its count, so it and the `(N!)` badge on the tab handle are one claim in two
  renderings — the badge says how many wait on the user, the heading says which they are.
- Neither heading appears when nothing waits elsewhere, which is the ordinary case: the lists then
  render exactly as they did. A permanent heading taxes every ordinary glance to explain an
  occasional one.
- A heading and its spacer are ROWS THAT SELECT NOTHING, the convention `row` already carries (`id ==
  ""`, as the mail listing's "showing the last N" note uses). Every key that moves the selection now
  goes through `moveCursor`, which steps over them — otherwise `j`/`k` rest on a label and the detail
  pane has nothing to show.
- Both tabs. Agents and PRs both hold foreign rows and must teach one reading habit; the complaint
  named both and only one had prompted it.
- Which group a row belongs in is asked of the SAME predicate that admitted it (`agentVisible`,
  `prVisible`): in scope means local, listed anyway means it waits on the user elsewhere. A second
  derivation of "is this foreign" would drift from the one deciding what is shown at all.

## Parity

`sindri agent list` and `sindri pr list` gain the same sections. They are fleet-wide rather than
scoped, so what is left once the waiting rows are lifted out spans every repo: those rows go under
`Other repos:` rather than under a `Local:` heading that would be false of most of them. As in the
TUI, no headings appear unless something waits elsewhere, and the flat listing keeps its order
exactly — the grouping is the only thing that changes, and only when it has something to say.

## Non-goals

The rows themselves are unchanged: same columns, same order within each group, no indentation. Rows
that shifted sideways whenever an agent in another repo got stuck would cost more than the heading
buys, and the `needs you` marker each row already carries stays where it is.

## Impact

- Specs: `view-tui`'s scope-toggle requirement gains the labelling and the non-selectable rule;
  `view-workers` gains the CLI half.
- Code: `internal/api/foreign.go` (new, the wording), `internal/ui/tui/rowsections.go` (new),
  `internal/ui/tui/tab_agents.go`, `tab_prs.go`, `items.go`, `onkey.go`, `viewport.go`, `util.go`,
  `internal/ui/cli/foreignrows.go` (new), `agent.go`, `hub.go`.
- Three badge tests now count the rows a cursor can rest on rather than every rendered line. The
  invariant is unchanged — a badge equals the items beneath it — but a grouped list also carries the
  labels naming them, and a heading was never an item.
