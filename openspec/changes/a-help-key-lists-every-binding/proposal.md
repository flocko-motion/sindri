# A help key lists every binding, and the prefix is advertised where it applies

## Why

The space prefix works on every tab, but its footer entry only ever appeared in the
tab-local row — advertising it as if it were a Tasks thing, or a PRs thing, which is the
opposite of true.

The footer also has no way to answer "what exists here" — only "what applies right
now". `U` unassign only shows while an agent holds the selected task, `A`/`R` approve
and reject a task only while a verdict is pending: real bindings, invisible almost all
the time, with no moment at which a user discovers them. Abbreviating the footer under
width pressure (sd-ac8831) makes this worse, not better, unless there is somewhere else
to look.

Separately, `N` on Tasks, Agents and Runs was direct while `N` on Chat already committed
("new meeting") — the same letter behaving two ways depending on the tab.

## What changes

- **`?` opens a reference modal**, leftmost in the global row — the first thing a reader
  sees, and the one key worth finding once the row has to shed the rest. It is a view,
  so it stays direct, outside the prefix.
- **The modal is generated from the keymap**, like the footers, so a binding and its
  help cannot drift apart.
- **The modal shows every binding for the tab, filtered differently from the footer.**
  The footer answers "what can I press now" and rightly hides what does not apply; the
  modal answers "what exists on this tab" and would make rarely-valid keys permanently
  invisible if it filtered the same way. So a conditional binding is always listed, with
  its condition spelled out in words ("unassign (while an agent holds it)") — taken from
  the keymap's own `when`, not written twice.
- **The prefix moves to the global row.** It works on every tab, so it is named once,
  globally, rather than repeated in every tab-local row. `?` leads the row and the
  prefix trails it; both are pinned — the last two things shed — since each names where
  the rest still reads in full (the reference, the menu). Only the entries between them
  shed, whole, never mid-word, when the row cannot fit everything.
- **The reference also carries the footer's real gaps**: escape's clear-filters (global —
  its meaning does not vary by tab) and enter's default "expand to the full-screen
  modal" (declared per scope on Tasks, Agents, PRs, Runs and Mail, the same way Repos
  and Chat already declare their own enter) are genuine dispatcher keys the footer
  deliberately never advertised. Both get a `refOnly` keymap row so `?` is not silently
  narrower than the dispatcher, without the footer picking them up. Enter is NOT a
  single global row: its meaning is per-tab, so one entry would read wrong on the two
  tabs (Repos, Chat) where it means something else — the review's own finding, since a
  reference that describes a different world than the dispatcher fails its one job. The
  reference does not separately restate the modal's own scroll/close keys — those are
  chrome, already named on the hint line every such modal shows while open.
  `TestReferenceListsRefOnlyBindings`/`TestRefOnlyBindingsStayOutOfTheFooter` pin both
  halves of `refOnly`, and `dispatchKeyParts` widens the one-key-one-meaning guard to
  stop dropping multi-character key names ("enter") the way it dropped genuine prose
  ("C-h") — the hole this slipped through the first time.
- **`N` commits on Tasks, Agents and Runs**, matching Chat's own `N`. Which letters
  commit is a case-by-case judgement recorded on the binding, not a rule to derive
  (`keys.go`'s own standing note); this removes an inconsistency rather than creating
  one.

## Review round 3

- **`t` on the PRs tab had no keymap row at all** — genuinely missing, found by hand a
  third time. Fixed (it shows the linked task, gated on the fetched detail actually
  being this PR's own and naming a task), and a completeness guard closes the class:
  `TestEveryDispatchedKeyHasAKeymapRow` carries an explicit list of every literal key
  onKey dispatches per tab and checks each against the keymap — the completeness half
  `dispatchKeyParts` (round 2) never gave the conflict guard.
- **The modal answered the wrong question first at the default terminal size.** GLOBAL's
  14 rows led the reference, so at 80x24 all but one of Tasks' own bindings — U/A/R
  among them — sat below the fold. `helpLines` now puts the current tab first.
- **Config appeared twice on the Repos reference** (global row, Repos row) — invisible
  as a duplicate before since the footer/menu already fold a tab's committing rows into
  the global one, surfaced once the reference stopped filtering by `commits`.
  `helpRows` dedupes a (key, label) pair to whichever scope renders it first.
- **`s` (scope toggle) is now live on Runs and Mail**, not just advertised there. Both
  tabs' rows are genuinely `inScope`-filtered already (`runRows`, `mailShown` ->
  `mailVisible`); onKey's guard just never covered them. Pre-existing, so not obligated
  by this task, but `?` turns a stale footer entry into a documented one — fixed the
  guard rather than leaving it undecided.

## Review round 4

- **The spec text was false for Mail** — "repo scope shows only the active repo's
  entries" is true of Runs but not Mail, which deliberately keeps a message addressed
  to the user regardless of its repo (the unread marker counts it fleet-wide;
  sd-ac1757/sd-57e895 exist because of exactly this class of badge/list disagreement).
  Qualified the requirement, added a scenario and a test — nothing pinned this before.
- **`whenText` on the PRs `t` row leaked implementation state.** Reworded from "the PR's
  own detail is loaded and names a task" to "the PR has a linked task" — the durable,
  observable half; the predicate itself did not need to change.

## Review round 5

- **The config duplicate was diagnosed wrong in round 3, and half-fixed.** footerFor and
  globalFooter already skipped it, but menuOffers deliberately admits global committing
  bindings on every tab — so pressing space on Repos actually offered `E config` twice.
  The real fix is deleting the redundant Repos row (pure redundancy: the global one
  already reaches every tab's menu); the reference's own dedup stays as a guard past
  that, now preferring the global declaration rather than whichever renders first.
- **The reference contradicted contextFooter's own hint while the detail column had
  focus.** j/k/enter/g/y mean something else then (`rightFocusKeys`), and helpLines
  rendered the tab's un-focused bindings regardless. Both now render from the one
  table.
- **whenText on the PRs `t` row leaked an internal fetch race** — reworded to the
  observable half.
- **`TestEveryDispatchedKeyHasAKeymapRow`'s other direction was unguarded**: a keymap
  row promising a key onKey never acts on (the shape of round 3's Runs/Mail scope-toggle
  bug). `TestEveryKeymapRowHasADispatchCase` closes it as a class.
- **The globalreviewerflow_test.go fix asserted nothing about `err`**, though the whole
  point is that it stays nil deliberately — now pinned (`code == 0 || err != nil`).

## Review round 6

- **`reopen` on Tasks had no `when`**, though onKey only acts on it while the task is
  closed (`taskReopenable`) — offered on every task, a silent no-op on almost all of
  them, and the reference documented it as unconditional. Fixed
  (`when: model.taskReopenable`), and closed the class: `dispatchGates`
  (completeness_test.go) records, per row, the exact predicate onKey's own case
  actually gates on, reflect-compared against the keymap's `when` — the hole the other
  three completeness guards couldn't see (the key IS dispatched, the row IS listed for
  it; the missing piece was one level deeper, in what condition the dispatch needs).
- **The scope-toggle requirement was false on two counts**, the same shape as round 4's
  Mail finding recurring in the same requirement. The tab does not default to `global`
  (`newModel` sets `scopeRepo: true`); and "repo scope shows only the active repo's
  entries" is true only of Runs — Agents and PRs keep anything needing the user from any
  repo, the identical exception given to Mail alone. Fixed the requirement's two
  sentences and scenarios to match; kept the "Default is global" scenario name (archive
  needs it, since MODIFIED replaces the whole block) but retargeted it at explicit
  `global` selection, adding a new, honestly-named scenario for what the tab actually
  starts in.
- **`why no review` (PRs) was declared inside the Tasks block**, so keymap's array order
  put it ahead of `verify` in the PRs reference, against that group's own stated order.
  Moved it after `scrap`, alongside Tasks' own `why next`'s position relative to its
  verdicts.

Noted, not fixed here — each is pre-existing, and each is a behaviour question needing
its own task rather than a fix bundled into this one:

- A rejected-but-open task's revise loop (A/R) is unreachable from the TUI: `taskGated`
  (onKey's gate) admits `approval == "rejected"`, but `taskAwaitsVerdict` (the keymap's
  `when`) is narrower (`Open && approval == "pending"`), so the menu never offers either
  key on that state, and "?" describes the `when` rather than what onKey was written to
  do.
- `keyMerge`'s "approve then merge" branch (`openApproveMergeChoice`) is dead from the
  keyboard: `when: model.selPRApproved` excludes M from the menu until the PR is already
  approved, so the confirm-and-merge-together path the code comments describe cannot
  currently be reached that way.
- Shedding the global row drops the least-recently-declared entry, not necessarily the
  least useful one — at some widths that hides `quit`/`refresh` while keeping less vital
  entries. "?" covers the loss, so this is a judgement call rather than a defect, but
  worth a second look.

## Review round 7

- **Fixing the Runs/Mail scope-toggle guard (round 3) turned a stale label into a false
  one.** Once `s` actually worked on Runs, its "repo+needs-you" wording became an
  authoritative claim rather than dead advertising — and Runs has no needs-you exception
  (no `RunNeedsUser`, `runRows` filters on scope alone). `scopeName` now takes a
  `needsYou` flag; Runs passes `false` and reads plain "repo".
- **The MODIFIED spec block rewound two other pending changes.** Three unarchived
  deltas modify the same requirement; archive replaces the whole block, so writing mine
  from the old baseline alone would have dropped `repo-scope-keeps-what-needs-you` and
  `label-the-foreign-section`'s text (grouping, labelling, one-predicate-per-kind, "the
  narrow scope's label SHALL say what it does" — the very rule that would have caught
  the Runs finding above) if archived after either of those. Rebuilt the requirement as
  their full text plus this change's own additions, so all three now differ only by
  what they add.
- Tried renaming the archived "Default is global" scenario per round 6's own finding;
  `openspec validate` refuses it regardless (same "omits scenario" error). Kept the
  name, dropped only the apology inside its THEN clause.
- `dispatchGates`' comment overclaimed exact agreement with onKey for one known
  exception (`{0, keyApprove}`/`{0, keyReject}` vs the wider `taskGated`) — named it
  instead of leaving a reader to trust a false claim.

## Review round 8

- **Round 5's rightFocus fix overshot: it made the reference too NARROW.** Replacing
  the whole tab section with `rightFocusKeys` while focused dropped every other binding
  that still works under focus — U/A/R among them, the exact bindings the addendum
  exists to surface. Rebuilt as an overlay (`focusRows`) that renders every row as
  usual and relabels only the keys whose meaning actually changed, splitting a compound
  row (`j/k/g/G`, `y/Y`) between the part that changes and the part that doesn't. The
  same overlay closes the GLOBAL section's matching contradiction for free.
- Two comment/spec overclaims, both non-blocking: `dispatchGates`' comment claimed one
  shape (checked directly in onKey) for entries that are really menu-enforced; the
  spec's Mail sentence said the narrow scope keeps "exactly" what the unread marker
  counts, when it keeps a superset (every message addressed to the user, not just the
  unread ones). Both reworded to say what is actually true.

## Review round 9

- **`h`/`l` (Tasks fold) were disabled under focus, with no way to say so.** onKey
  gated both on `!m.rightFocus`; the reference (and the honest footer above it) kept
  advertising "h/l fold" as working regardless. Dropped the guard — folding needs no
  particular column focused — so the existing text becomes true, rather than adding a
  way to mark a binding "disabled here" that `rightFocusKeys` (built for keys whose
  meaning *changes* under focus) was never shaped to express.
- Two small nits: Tasks' "enter" row read as the other half of "h/l fold" in context
  ("expand" vs "fold") — renamed to "full screen". `keyEnter` existed to unify the
  string but two literal `"enter"`s survived next to it — now use the constant.

## Round 10: found during self-verification, before resubmitting

Re-deriving every onKey case against its keymap row — the exercise the reviewer runs
each round — surfaced eleven Agents-tab bindings (`tell`, `mail`, `attach`, `editor`,
`start/stop`, `options`, `milestone PR`, `rebuild image`, `rebase`, `retire`, `clear
context`) that silently no-op on an orphan container (a stray pod with no roster entry,
also listed on the Agents tab) via `selAgent`/`isOrphan`, none of it reflected as a
`when` — the same shape as round 6's `reopen` and round 9's `h/l`, just eleven rows at
once. Added `agentSelected` and gated all eleven, extended `dispatchGates` to match, and
added tests pinning both sides (orphan hides them, a real agent doesn't) plus the actual
dispatch behaviour (silent, not broken).

## Review round 11

- **Shedding dropped whole entries in declaration order, which is not usefulness
  order.** At 80 columns, basic movement (`j/k/g/G`) was gone while tab-jump and
  pane-switching survived, and `q` quit was invisible below 200 columns — the opposite
  of sd-5e3032's "shed the least useful entries". Added a usefulness ranking
  (`globalShedOrder`) independent of keymap's own reading order, and fixed the spec
  delta, which had dropped the *which* entirely and only stated the *how*.
- **Comments across the touched files anchored themselves to review-round history** —
  "is the review finding", "round 9", "2.10's first fix" — none of which resolves once
  this lands. Reworded roughly thirty of them to state the durable invariant instead,
  keeping bare task IDs where they point somewhere real.
- Three small non-blocking fixes: `focusRows`' relabelled branch documented and pinned
  (it deliberately skips `formatRow`, which is only safe while nothing focus-remaps
  also commits or carries a `when`); `scopeName`'s boolean-blind call sites replaced
  with one derived from the active tab, so the wrong-literal mistake round 7 caught
  once cannot recur; and a note that the 80x24 fold has no spare room left.

## Review round 12

- **BLOCKER: the live global footer, not just the reference, was never made
  focus-aware.** Round 8 fixed `helpRows`/`focusRows` so the "?" modal relabels j/k/g/y
  correctly while the detail/meta column has focus — but `globalFooter`, rendered on
  screen the whole time, was untouched, so its row still read `j/k/g/G move/top/bot`
  under focus while the tab-local row right below it read `j/k item · g goto · y copy`:
  the TUI's own two footer rows disagreed with each other on screen. Extracted
  `focusSplit` as the one shared primitive both `globalFooter` and the reference now
  use, so the two cannot drift apart again. Verified the regression test
  (`TestGlobalFooterAgreesWithTheLocalRowWhileFocused`) actually catches this by
  reverting the fix and watching it fail before restoring it.
- **The reference restates the modal's own chrome keys, but hiding them would break an
  earlier, deliberate requirement.** `j/k/g/G`, `C-d/C-u`, `esc` and `q` all read their
  ordinary GLOBAL meaning in the reference, directly above the modal's own accurate
  hint line — and while the reference is open, those same keys really do scroll/page/
  close it instead. But `esc` IS the modal's close key, and an earlier round wrote the
  requirement (and `TestReferenceListsRefOnlyBindings`) specifically to guarantee esc's
  row stays visible there. Hiding "modal chrome" rows would have deleted esc's row and
  broken that guarantee. Reworded the spec sentence to say what was actually meant: no
  *dedicated* row for the modal's own chrome (none exists, and the hint line already
  covers it), which does not reach into hiding an ordinary GLOBAL row's everyday
  meaning.
- Non-blocking, fixed: `dispatchGates`' partner test doc-comment implied it closed the
  whole class of `when`-vs-onKey gating bugs; it does not cover "is anything selected
  at all", which the reviewer explicitly said was not being asked to be newly closed —
  only documented. Reworded the comment to say so.
- Non-blocking, fixed: Tasks' and PRs' attach flashed "no agent is working " with
  nothing appended when nothing was selected (`selID()` returns `""`) — a sentence cut
  off mid-word. Added `noAgentFlash`, verified against the original bug the same way as
  the blocker above (revert, watch the new test fail, restore).
- Two small nits: the help modal's title ("Help — Tasks") duplicated the body's own
  first line one line below it — simplified to just "Help". `globalShedOrder` (round
  11) had ranked "tab" as its second-least-useful entry, dropping it early at 80
  columns despite switching tabs being a common action, not a rare one — promoted it
  to survive alongside movement and quit.
- Recorded, not changed: `shedMiddle`'s final fallback skips its own shed-marker in
  favour of `ansi.Truncate`'s, reachable only once lead+trail alone overflow a terminal
  too narrow for any footer to be useful — noted in a comment rather than special-cased
  for a case that cannot occur in practice.

## Review round 13

- **SPEC VIOLATION: the global row shed basic movement and quit before the
  focus-remapped entries, under focus.** `globalShedOrder` ranks by label, but
  `focusSplit` mints labels it has never heard of ("item", "goto", "copy"), so
  `shedPriority`'s not-found fallback sorted them LAST — kept longest, ahead of
  actual named entries. At 80 columns focused, movement (`G move/top/bot`) was shed
  while `y copy` survived; the opposite of "basic movement and quitting SHALL be
  among the last entries shed". Fixed by ranking a `globalEntry` by its PARENT
  binding's own label (`rank`), not the display label focus happens to remap it to
  — a focus-split part now sheds exactly when its unsplit binding would.
  `TestGlobalFooterShedsByUsefulnessUnderFocusToo` pins it, verified against the
  original bug the same way as prior rounds (revert, watch it fail, restore). The
  duplication-with-contextFooter symptom the reviewer also flagged needed no
  separate fix — it was the same ranking bug wearing a different face.
- Non-blocking, fixed: `shedPriority`'s comment claimed an unlisted entry is "never
  shed"; the loop actually reaches it last, not never. Reworded.
- Non-blocking, fixed: two comments (the `keyHelp` constant's own doc comment, and
  onkey.go's dispatch case for it) described the modal's section order backwards —
  "global, then the current tab's" when the code, and the spec, put the tab first.
  Fixed the wording to match the code rather than the reverse, since a change whose
  whole point is "a binding and its help cannot drift apart" cannot ship a comment
  that drifts from both.

## Also fixed in this branch, then superseded upstream

`internal/hub/workflow/globalreviewerflow_test.go`'s `TestShowIsScopedUnlessTheCallerHoldsTheNamedPR`
asserted a non-nil error from `CmdShowPR`'s not-found path, which deliberately returns `(1, nil)` —
its own comment explains why (a nil error there is user-facing, not a hub fault; a real error would
be masked by `AgentExec` as "an internal error"). The test was red on an otherwise-unrelated base;
fixed to restore the real regression check (no leak of the foreign PR's task/branch/agent) instead
of the wrong assertion, and later pinned the nil-error side too (review round 5).

A subsequent rebase pulled in `fix: a refused PR is an answer, not a hub fault`, which lands the same
contract upstream, more broadly, including this exact test. `sindri git change` on the file now shows
nothing — this branch's own copy of the fix is a no-op post-rebase, kept in the task history so a
reader is not left looking for a diff that isn't there.

## Impact

- Specs: `view-tui` gains the reference-modal requirement and the prefix's global
  placement.
- Code: `internal/ui/tui/keys.go` (the `?` binding, `whenText`, `globalFooter`,
  `helpLines`/`helpRows`), `items.go` (`openHelpModal`), `onkey.go` (the dispatch),
  `tui.go` (the footer composition).
- Not a parity change: a view, generated from state the TUI already holds. No new hub
  call.
