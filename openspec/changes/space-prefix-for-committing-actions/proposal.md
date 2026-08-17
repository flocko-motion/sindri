# Put committing actions behind a space-prefix menu

## Why

The hotkey surface outgrew the footer. Each tab carried about a dozen bindings on top of nine global
ones, and eight working bindings — J/K, g/G, y/Y, h/l, ctrl+d/u — were never advertised at all
because there was no room. Adding an action meant hunting for a free letter and squeezing another
entry into a row nobody reads.

It is also a safety story. `TestMergeIsUppercase` existed because merge "commits on the keystroke —
no form, no chooser — so it is the one action that most needs the shift". A shift is a thin guard.

## What changes

- **SPACE is a prefix, like tmux's.** It opens a menu that both shows the committing actions and
  activates their letters: space, then M to merge. Bare M does nothing. Escape cancels. The letters
  are unchanged, so nothing is unlearned — there is one deliberate keystroke in front.
- Not ctrl+a or ctrl+b: this TUI runs inside tmux routinely, and attaching to an agent hands the
  terminal to another tmux session, so an outer tmux swallows ctrl+b before sindri sees it and
  ctrl+a collides with screen. Space is unbound, shadows no navigation, and needs no shift.
- **The menu is generated from the keymap**, which already held key, label and scope and already
  generated the footers. A binding gains `commits` (does the keystroke change state) and `when`
  (does it apply to this row). One table drives dispatch, footer and menu.
- **It is context-filtered**: no merge on an unapproved PR, no unassign on a task nobody holds, no
  reopen on an open one, no verdict on a task nobody proposed. That is what makes it better than a
  flat list — it answers "what can I do with this". The hub already does this for an agent's command
  surface, where an out-of-order verb is invisible rather than refused.
- **Inertness does not depend on the selection.** A key that commits anywhere on the tab is never
  live bare; otherwise whether a stray press did something would depend on which row was selected.
- **The convention now reads NAVIGATE OR COMMIT, with no exception clause.** Opening an editor,
  form, prompt or picker is navigation — you have moved to a place where you may act, and the act
  happens on submit inside it. The old wording needed a carve-out ("it may OPEN a form … since that
  flow is confirmable"); the new framing needs none, because opening a form was never a mutation.
- **The line is drawn by what the keystroke does, not by the letter's case**, per binding and per
  tab. So `e`, `i`, `t`, `c`, `o`, `a` stay direct — and so do `N` (task form), `O` (options/reopen
  form), `P` (priority picker), `E` (config form), `B` (planner picker), `I` (review form), and `R`
  where it opens a rejection form or a member picker. They are destinations exactly as `c` is, and
  the case they carry is a legacy of the old rule. The same letter can differ by tab: `A` commits on
  the PRs tab and opens a picker on the Meeting tab, `R` rebases an agent on the keystroke and
  opens a form on a PR.
- **A confirm is part of the commit, not a destination.** It asks "sure?" about an action the
  keystroke has already chosen, so `D` (scrap/delete/forget), `M` (milestone), `B` (rebuild) and the
  clear-context `C` are behind the prefix even though a modal follows. Anything else would leave a
  stray `D` popping a delete-agent confirm.
- **The footer carries the navigating keys plus one prefix entry**, reading "space actions" — or
  "space actions (none here)" on a row with nothing to offer, since a bare "actions" leading to an
  empty box reads as a bug. It is shorter than before, but NOT the collapse to "navigation plus one
  entry" the body pictured: the destinations that stay direct are still advertised there, so the
  Tasks footer still runs to about ten entries. See the note below.

## What this does not buy, and the choice behind it

The body expected the footer to collapse far enough to make room for the movement keys sd-ac8831
must advertise. Drawn as the body's own rule requires — opening a form or picker is navigation, so
it stays direct — most of the Tasks row survives: `N new · h/l fold · B brief a planner · i comment
· a attach · e edit · P priority · O reopen · n why next · f filter · space actions`.

The alternative is to gate every path that reaches a mutation, confirmable or not, which puts
N/O/P/E/B/I behind the prefix and leaves a genuinely short footer. That is the position of one
earlier comment on the task, which a later one explicitly overruled, and it is a design decision
rather than an implementation detail — so this change follows the settled body, and the choice
stays with the user.

## The pinned tests, revisited deliberately

- `TestMergeIsUppercase` became `TestMergeIsBehindThePrefix`: M is still merge, it is declared
  committing, the menu offers it on an approved PR, and the footer no longer does. Its sibling adds
  what the prefix makes possible — merge is absent on an unapproved PR, and pressing it does nothing.
- `TestPRTabVerdictKeys` asks the menu for A/R/I and the footer for `e editor`, which is where each
  now belongs.
- `TestLowercaseKeysNeverMutate` keeps its allowlist, fail-closed by design. `TestNothingLowercase
  Commits` is the structural half beside it: no lowercase binding may be declared committing, and
  that one needs no list to maintain.
- `TestNoTwoActionsShareAKeyOnATab` still holds as written — one key, one meaning per tab, whether
  the key is reached directly or through the menu.
- `TestConfirmModalsDefaultToCancel` and `TestOldKeysLostTheirOldJobs` are untouched: the modals and
  the rebindings they pin are unchanged by the prefix.

## Impact

- Specs: `view-tui` gains the prefix requirement.
- Code: `internal/ui/tui/keys.go` (the `commits`/`when` fields, the reworded convention, the
  footer), `component_menu.go` (new: the offers, the availability rules, the box), `onkey.go` (the
  gate), `tui.go` (the model flag and the overlay).
- Not a parity change: every one of these actions already exists as a CLI command. TUI ergonomics.
