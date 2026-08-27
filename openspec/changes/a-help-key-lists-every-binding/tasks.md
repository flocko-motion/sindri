# Tasks

## 1. The space prefix is advertised globally, not locally (sd-5e3032)

- [x] 1.1 `footerFor` no longer appends the prefix entry to a tab-local row.
- [x] 1.2 `globalFooter` pins `?` leading and the prefix entry (`menuLabel` for the
      current tab) trailing; `shedMiddle` drops whole entries — never mid-word — from
      between them when the row cannot fit everything.
- [x] 1.3 `View()` composes the footer from `globalFooter`, not `footerFor(scopeGlobal)`.
- [x] 1.4 Review fix: shedding from the tail (as first written) dropped the prefix
      first, since it was appended last — invisible below ~180 columns on most tabs,
      where it used to fit the tab-local row comfortably. `TestGlobalFooterKeepsHelp
      AndPrefixAtOrdinaryWidths` pins both ends surviving at 80/100/120 columns.

## 2. `?` opens a reference modal (sd-5e3032)

- [x] 2.1 `keyHelp = "?"` leads the global row, direct (not behind the prefix) — it is a
      view.
- [x] 2.2 `binding` gains `whenText`, spelling a `when` predicate out in words; every
      `when`-gated binding in the keymap carries one, held by
      `TestEveryWhenGatedBindingHasWhenText`.
- [x] 2.3 `helpLines`/`helpRows` render every binding in scope — global, then the current
      tab's — unfiltered by `when`; a conditional binding is listed with its condition,
      and a committing one with the prefix folded into its key ("space N").
- [x] 2.4 `openHelpModal` opens it in the same scrollable modal every other detail view
      uses (`modalOverride`/`m.modal`).
- [x] 2.5 Review fix, round 1: the reference only listed keymap rows, so escape's
      clear-filters and enter's default expand-to-modal — real dispatcher keys the
      footer deliberately never carried — stayed as undiscoverable as before. Both get
      a `refOnly` keymap row: listed in `?`, skipped by `footerFor`/`globalFooter`.
- [x] 2.6 Review fix, round 2: the `refOnly` "enter — expand" row was declared globally,
      which is factually wrong on Repos ("switch") and Chat ("compose") and shows both
      readings at once in the reference. Declared per scope instead (Tasks, Agents, PRs,
      Runs, Mail), the same way Repos and Chat already declare their own enter.
      `dispatchKeyParts` (keys_test.go) widens `TestNoTwoActionsShareAKeyOnATab`'s guard
      to stop dropping multi-character key names ("enter") as if they were prose — the
      hole this slipped through. `TestReferenceListsRefOnlyBindings`/
      `TestRefOnlyBindingsStayOutOfTheFooter` pin the `refOnly` invariant itself, which
      round 1 shipped with no guard of its own.
- [x] 2.7 Review fix, round 3: `t` on the PRs tab (show the linked task, onkey.go's
      `keyTell` case) had no keymap row at all — genuinely missing, found by hand a
      third time. Added the row (`when: prShowsLinkedTask`), and
      `TestEveryDispatchedKeyHasAKeymapRow` (completeness_test.go) closes the class of
      bug: an explicit, commented list of every literal key onKey dispatches per tab,
      cross-checked against the keymap — the completeness half `dispatchKeyParts` never
      gave the conflict guard. `dispatchKeyParts` itself gained a fix along the way:
      `keySearch` ("/") was being split on itself and vanishing.
- [x] 2.8 Review fix, round 3: at 80x24, GLOBAL's 14 rows led the reference, so Tasks'
      own section — U/A/R among them, what the addendum was written for — mostly sat
      below `modalContentHeight(24)`'s fold. `helpLines` now puts the current tab first,
      GLOBAL after; `TestHelpModalShowsTheCurrentTabWithinTheFold` pins it visible.
- [x] 2.9 Review fix, round 3 (corrected round 5 — see 2.11): config (global, commits)
      was also declared on Repos, listed twice once the reference stopped filtering by
      `commits`. `helpRows` dedupes a (key, label) pair to whichever scope renders it
      first; `TestHelpModalNeverListsTheSameBindingTwice` pins it.
- [x] 2.10 Review fix, round 5: `?` contradicted contextFooter's own rightFocus hint —
      helpLines rendered the tab's un-focused bindings for j/k/enter/g/y while the
      detail/meta column had focus, when those keys mean what `rightFocusKeys` says
      instead. `rightFocusKeys` (items.go) is now the one source both contextFooter and
      helpLines render from; `TestHelpModalMatchesTheFooterWhileFocused` pins it.
- [x] 2.11 Review fix, round 5 — BLOCKER: 2.9's diagnosis was half wrong. footerFor and
      globalFooter did already skip the duplicate (they skip every committing binding
      from a tab-local list), but menuOffers deliberately admits global committing
      bindings on every tab (its own comment says so), so it offered config twice and
      menuFooter rendered both — "E config · E config" on Repos, unpinned by any test.
      The actual fix is deleting the redundant Repos row (pure redundancy: scopeGlobal
      already reaches every tab's menu); helpRows' dedup stays as a guard past that, not
      the fix, and now prefers the global declaration when one of the two is global (a
      tab-scope duplicate is excluded, not whichever renders first).
      `TestReposMenuOffersConfigOnce` pins the actual bug; `TestHelpModalNeverListsThe
      SameBindingTwice` now also asserts config is attributed to GLOBAL.
- [x] 2.12 Review fix, round 5, nit: `TestEveryKeymapRowHasADispatchCase` closes
      `TestEveryDispatchedKeyHasAKeymapRow`'s other direction — every non-`when`-gated
      row's dispatched key must appear in `dispatchedKeys` too, catching a row that
      promises a key onKey never acts on (round 3's 5.1 shape) as a class, not an
      instance. Fixed `dispatchKeyParts` along the way: the decorative "⇥" (tab glyph)
      survived its rune-count filter but is never itself a real dispatched key.

## 3. New commits consistently across tabs (sd-5e3032)

- [x] 3.1 `keyNew` commits (`commits: true`) on Tasks, Agents and Runs, matching the
      Chat tab's existing "new meeting".

## 4. Unrelated fix carried on this branch, then superseded upstream

- [x] 4.1 `TestShowIsScopedUnlessTheCallerHoldsTheNamedPR` (globalreviewerflow_test.go,
      from the earlier reviewers-are-a-global-pool feature) was red on an otherwise-
      unrelated base: it asserted a non-nil error from `CmdShowPR`'s not-found path,
      which deliberately returns `(1, nil)` per its own comment. Fixed the assertions to
      match and to check for the PR's actual content leaking, not the echoed id.
- [x] 4.2 Review fix, round 5, nit: that fix (4.1) asserted nothing about `err` at all,
      so the very contract it argues for — a nil error on this path, deliberately — was
      described in the test's comment but not pinned. Fixed to assert it explicitly.
- [x] 4.3 A later rebase (`fix: a refused PR is an answer, not a hub fault`) landed the
      same contract upstream, more broadly, including this exact test. This branch's own
      copy of 4.1/4.2 is now a no-op — `sindri git change` on this file shows nothing —
      so nothing of it actually ships in this PR; recorded so a reader of this history
      doesn't go looking for a diff that isn't there.

## 5. Pre-existing, carried on this branch by decision (review, round 3, item 4)

- [x] 5.1 `s` (scope toggle) was advertised on Runs and Mail — with a label rendering
      live state, reading as working — since before this feature, but onKey's guard
      only acted on Agents and PRs. Predates this branch, so not obligated here, but
      `?` now presents the keymap as the authoritative reference, turning a stale
      footer entry into a documented one; leaving it undecided was not an option.
      Decision: fix the guard, not drop the rows — both tabs' rows are genuinely
      `inScope`-filtered (`runRows`, `mailShown` -> `mailVisible`), so the feature
      already works, it just needed the keystroke wired.
      `TestScopeToggleWorksOnRunsAndMail` pins it.
- [x] 5.2 Review fix, round 4 — BLOCKER: the spec text carried over verbatim ("repo
      scope shows only the active repo's entries") is true of Runs but false of Mail:
      `mailVisible` deliberately keeps a message addressed to the user regardless of
      its repo, since the unread marker counts it fleet-wide (sd-ac1757/sd-57e895) — a
      spec that told the next reader to scope it away would reintroduce that exact bug.
      Qualified the MODIFIED requirement, added a scenario, and
      `TestMailToUserSurvivesRepoScope` pins the behaviour a test never had before.
- [x] 5.3 Review fix, round 4, non-blocking: the PRs `t` row's `whenText` named an
      internal fetch race ("while the PR's own detail is loaded and names a task") the
      reader cannot observe or act on. Reworded to the durable half: "while the PR has
      a linked task". `prShowsLinkedTask` itself is unchanged — only the wording leaked.

## 6. Review fixes, round 6

- [x] 6.1 BLOCKER: `reopen` (Tasks) had no `when`, though onKey only acts on it while
      the task is closed (`taskReopenable`) — offered on every task, a silent no-op on
      almost all of them, documented by "?" as unconditional. Fixed
      (`when: model.taskReopenable, whenText: "while it's closed"`). Closed the class:
      `dispatchGates` (completeness_test.go) reflect-compares each `when`-gated row
      against the exact predicate onKey's own case checks — the hole one level deeper
      than the other three completeness guards, which only see "is the key dispatched"
      and "is the row listed", not "does the row's condition match the dispatch's own".
- [x] 6.2 BLOCKER: the scope-toggle requirement was false on two counts — defaulting to
      `global` (the tab actually starts in `repo`), and "repo scope shows only the
      active repo's entries" being true only of Runs (Agents and PRs keep anything
      needing the user, the identical exception already given to Mail alone). Fixed
      both sentences; kept the archived "Default is global" scenario name (MODIFIED
      replaces the whole block) but retargeted it at explicit selection, and added an
      honestly-named scenario for what the tab actually starts in, plus one for the
      Agents/PRs needs-you exception alongside Mail's own.
- [x] 6.3 Non-blocking, fixed: `why no review` (PRs) was declared inside the Tasks
      block, so keymap's array order put it ahead of `verify` in the reference, against
      the PRs group's own stated order. Moved after `scrap`, mirroring where Tasks'
      own `why next` sits relative to its verdicts. `TestWhyNoReviewSitsInThePRsBlock`
      pins the order.
- [x] 6.4 Non-blocking, one of three fixed here; the other two are behaviour questions and
      are filed as their own tasks, per the reviewer's own framing.
      FIXED: A/R unreachable on a rejected-but-open task. `taskGated` (pending OR
      rejected) is what onKey gates on and what the gate actually holds, so the keymap
      row now carries it too — the rejected branch was unreachable, since the menu
      refuses a letter it never offered. `TestARejectedTaskStillOffersAVerdict` pins it,
      with both boundaries (pending still offers, closed and approved do not).
      DEFERRED, sd-9b34c1: `keyMerge`'s approve-then-merge branch dead from the keyboard.
      Widening the gate is a real decision rather than a repair — `TestMergeIsNotOffered\
      OnAnUnapprovedPR` asserts today's behaviour deliberately, and the safety argument
      (no keyboard route to merging an unreviewed PR) stands against the dead-path one.
      DEFERRED, sd-6e972c: shedding drops declaration order, not usefulness order.

## 7. Review fixes, round 7

- [x] 7.1 BLOCKER: fixing 5.1's guard (round 3) made "s scope: repo+needs-you" LIVE on
      Runs, and it is false there — `runRows` filters on `inScope` alone, no
      `RunNeedsUser` anywhere. `scopeName` now takes a `needsYou` flag: Agents, PRs and
      Mail pass `true` (unchanged wording); Runs passes `false` and reads plain "repo".
      `TestRunsScopeLabelDoesNotClaimNeedsYou` pins it — fixing the guard without fixing
      the label would have been half the decision, exactly as flagged.
- [x] 7.2 BLOCKER: the MODIFIED block was written from the pre-existing archived
      baseline alone, silently rewinding two other unarchived changes' text for the same
      requirement (`repo-scope-keeps-what-needs-you`, fully carried forward by
      `label-the-foreign-section`) — the foreign-group heading/labelling/ordering
      behaviour, the one-predicate-per-kind rule, the "narrow scope's label SHALL say
      what it does" rule (the one that would have caught 7.1), and six of their
      scenarios. Rebuilt the requirement as their full text plus this change's own
      additions (Runs/Mail sharing the toggle, Mail's identical exception, Runs' plain
      label, the corrected default), so the three deltas now differ only by what each
      adds, not by what each silently drops.
- [x] 7.3 Attempted, reverted: renaming "Default is global" (kept since round 5, tested
      as false by round 6's own reviewer) — `openspec validate` still refuses it,
      rejecting the rename with the same "omits scenario" error regardless of framing.
      Kept the name, dropped only the apologetic parenthetical from its THEN clause.
- [x] 7.4 Nit, non-blocking: `dispatchGates`' comment claimed exact agreement with
      onKey's own gate for every entry, but `{0, keyApprove}`/`{0, keyReject}` record
      `taskAwaitsVerdict` while onKey's real gate (`taskGated`) is wider (also admits a
      rejected task) — the one known divergence the guard doesn't close, per 6.4. Named
      the exception in the comment rather than silently trusting it matched.

## 8. Review fixes, round 8

- [x] 8.1 BLOCKER: 2.10's rightFocus fix overshot — `helpLines` REPLACED the tab
      section with `rightFocusKeys` while focused, instead of overlaying just the keys
      whose meaning actually changed. Everything else the tab dispatches (U/A/R among
      them — the exact bindings the addendum exists to surface) vanished from the
      reference while still working. Rebuilt as an overlay: `helpRows`/`focusRows` now
      render every row as usual and, under focus, split a row between the keys
      `rightFocusKeys` covers and the keys it doesn't (a compound row like `j/k/g/G`
      or `y/Y` mixes both — `G` and `Y` are unaffected by focus and keep their ordinary
      label). `TestHelpModalMatchesTheFooterWhileFocused` is inverted (unassign must
      now be PRESENT), and `TestHelpModalStillWorksAlongsideTheMenuWhileFocused`
      reproduces the reviewer's own repro (N/C/D genuinely offered and listed while
      focused, on an open task).
- [x] 8.2 Non-blocking, fixed as part of 8.1: the GLOBAL section's own contradiction
      (`j/k/g/G move/top/bot`, `y/Y yank/all` unconditionally, while the tab section
      said something else) is closed by the same overlay — `helpRows` applies it to
      whichever scope it renders, GLOBAL included.
- [x] 8.3 Nit, non-blocking: `dispatchGates`' comment still overclaimed after 7.4 —
      `{0,keyUnassign}`, `{0,keyClose}` and `{2,keyApprove}` commit, so onKey's own case
      runs unconditionally once the menu has already gated the keystroke; the recorded
      predicate is what the MENU enforces, not a second check inside onKey. Split the
      comment into the two real shapes (checked directly vs. menu-enforced) instead of
      claiming one shape for both.
- [x] 8.4 Nit, non-blocking: the spec's new Mail sentence claimed a message addressed to
      the user is "exactly" what the unread marker counts — false; `mailVisible` keeps
      every such message, read or not, while the marker counts only the unread (a
      superset, not an equality). Reworded to say what is actually true.

## 9. Review fixes, round 9

- [x] 9.1 BLOCKER: `h`/`l` (Tasks fold) had no `when` and no focus handling, but onKey
      gated both on `!m.rightFocus` — a silent no-op while the detail/meta column had
      focus, which the reference (and the honest footer above it) kept advertising as
      working. Fixed by dropping the guard: folding the row under the cursor needs no
      particular column focused, so the existing text becomes true rather than growing
      new machinery to express "disabled here". `TestFoldWorksUnderFocus` pins it.
- [x] 9.2 Non-blocking, fixed: Tasks' "enter  full screen" row (was "expand") read as
      the other half of the "h/l fold" pair six lines away, in context. Renamed to
      avoid the collision.
- [x] 9.3 Non-blocking, fixed: `keyEnter` exists "so every use agrees on the string",
      but onkey.go's own `case "enter":` and component_modal.go's `case "esc", "enter",
      "q":` still spelled it out. Both now use the constants.

## 10. Self-found during round 10 verification, before resubmitting

- [x] 10.1 Re-deriving every onKey case against its keymap row (the same exercise the
      reviewer runs each round) surfaced eleven Agents-tab bindings that silently no-op
      on an orphan container — a stray pod with no roster entry, also listed on this
      tab, routed only to its own removal (`D`). `selAgent`/`isOrphan` gate `tell`,
      `mail`, `attach`, `editor`, `start/stop`, `options`, `milestone PR`,
      `rebuild image`, `rebase`, `retire` and `clear context` in onKey, none of it
      reflected as a `when` — the same shape as round 6's `reopen` and round 9's `h/l`,
      just eleven rows at once instead of one. Added `agentSelected` and gated all
      eleven; `dispatchGates` gained the same entries so this class stays closed;
      `TestAgentBindingsHideOnAnOrphanRow`/`TestAgentBindingsSurviveOnARosterAgent`/
      `TestAttachIsSilentNotBrokenOnAnOrphan` pin both the orphan and the roster-agent
      side, plus the actual dispatch behaviour.

## 11. Review fixes, round 11

- [x] 11.1 BLOCKER: `shedMiddle` dropped whole entries off the tail of keymap's
      declaration order, which is the READING order sd-5e3032 chose it for, not the
      usefulness order the WIDTH section asked shedding to follow ("shed the least
      useful entries"). At 80 columns — the default terminal — `j/k/g/G move/top/bot`
      was gone while `1-7 jump` and `C-h/C-l pane` survived, and `q quit` was invisible
      below 200 columns. Added `globalShedOrder`, a usefulness ranking independent of
      declaration order, consulted by a rewritten `shedMiddle`: it now sheds by that
      ranking (least useful first) while what survives still renders in keymap's own
      order. Also fixed the spec delta, which stated the *how* (shed whole entries) but
      dropped the *which* (least useful) entirely — the requirement text and a new
      scenario now carry it. `TestGlobalFooterShedsByUsefulnessNotDeclarationOrder` pins
      that movement and quit survive at 80 columns while jump and pane-switching go
      first.
- [x] 11.2 Comment cleanup: roughly thirty comments across the touched files anchored
      themselves to review-round history ("is the review finding", "round 3, item
      5.1", "2.10's first fix REPLACED...", "sd-a6e884 round 9") — none of it resolves
      for a reader once this lands, since there is no round history to look up outside
      this PR. Reworded each to state the durable invariant instead, keeping bare task
      IDs where they name something a reader can actually look up (sd-5e3032,
      sd-ac1757, sd-6d0ff2, tasks.md's own numbered sections). The round-by-round
      history stays here, in full, which is what this file is for.
- [x] 11.3 Non-blocking, fixed: `focusRows`' relabelled branch renders the focused
      label directly rather than through `formatRow`, since it describes a different
      action from whatever the binding does un-focused — deliberate, but only safe
      while no binding whose keys intersect `focusOverrides` also commits or carries a
      `when`. Documented the reasoning and added
      `TestFocusOverriddenKeysNeverCommitOrCarryAWhen` to hold the invariant rather than
      leave it silently assumed.
- [x] 11.4 Non-blocking, fixed: `scopeName`'s `needsYou` bool was a literal at each of
      four call sites, one already caught wrong once (round 7). Replaced it with
      `scopeNeedsYou(scope)`, read off `tabScope(m.tab)` inside `scopeName` itself —
      every call site is now identically `scopeName(m.scopeRepo, m)`, so a future
      scope-toggle row cannot pass the wrong literal because there is no literal to pass.
- [x] 11.5 Non-blocking, recorded: at 80x24, Tasks' and Agents' own sections are each
      exactly `modalContentHeight(24)` lines — no spare room. Not fixed (no current
      binding is missing), but the fold test's comment now says the budget is
      exhausted, so the next binding added to either tab comes with a conscious
      width/height check instead of a silent assumption that the fold still holds.

## 12. Review fixes, round 12

- [x] 12.1 BLOCKER: round 8 made the "?" reference focus-aware (`focusRows`), but
      `globalFooter` — the LIVE global footer row, on screen the whole time — was
      never touched. While the detail/meta column had focus, the global row still
      rendered `j/k/g/G move/top/bot` verbatim, contradicting the tab-local row right
      below it (`j/k item · g goto · y copy`) and disagreeing with onkey.go's own
      j/k/g/y cases, which all branch on `rightFocus`. Fixed by extracting `focusSplit`
      out of `focusRows` as the one shared primitive both the footer and the reference
      now call, so the two on-screen rows cannot describe different worlds again.
      `TestGlobalFooterAgreesWithTheLocalRowWhileFocused` pins it; confirmed it catches
      the regression by reverting `globalFooter` to the unfixed form and watching the
      test fail before restoring the fix.
- [x] 12.2 Non-blocking, resolved by tightening the spec, not the code: the reference's
      GLOBAL section restates `j/k/g/G`, `C-d/C-u`, `esc` and `q` with their ordinary
      meanings, sitting right above the modal's own accurate hint line — which, while
      the reference itself is open, is also literally what those keys do (scroll/page/
      close the modal). Actually hiding those rows from the GLOBAL section would
      contradict the very sentence above it that requires `esc`'s row to exist
      (refOnly's whole point, pinned by `TestReferenceListsRefOnlyBindings`) — esc IS
      the modal's own close key, so a rule reading "never restate modal chrome" cannot
      mean what it seemed to. Reworded the spec sentence to say what was actually
      meant: the modal gets no *dedicated* row for its own scroll/close chrome (there
      isn't one, and none is needed — the hint line already covers it), and that this
      does not hide an ordinary GLOBAL row's everyday meaning merely because the
      reference modal also answers to the same key while open.
- [x] 12.3 Non-blocking, fixed: `dispatchGates`' partner test doc-comment read as though
      matching a row's `when` to onKey's real condition closed the whole class of
      gating bugs. It does not cover whether a row is selected at all — a predicate can
      pass on an empty list exactly as onKey's own case does, and neither is asked to
      notice. Reworded `TestDispatchGatesMatchTheirKeymapRow`'s comment to say so,
      matching the reviewer's own explicit statement that this gap is not being asked
      to be newly closed, only documented.
- [x] 12.4 Non-blocking, fixed: Tasks' and PRs' `keyAttach` case flashed
      `"no agent is working " + m.selID()`, which reads as a sentence cut off mid-word
      when nothing is selected (`selID()` returns `""`). Added `noAgentFlash`, used by
      both call sites, so an empty selection reads "nothing selected" instead of a
      trailing space. `TestAttachFlashIsNotTruncatedWhenNothingIsSelected` pins it;
      confirmed it catches the original truncation by reverting to the inline
      concatenation and watching the test fail before restoring the fix.
- [x] 12.5 Non-blocking, fixed: the help modal's title, "Help — Tasks", duplicated the
      body's own first line ("Tasks") one line below it. Simplified the title to just
      "Help" — the tab name is already the first thing the reader sees in the body.
      `TestHelpKeyOpensAReferenceModal` updated to assert the plain title.
- [x] 12.6 Non-blocking, recorded: `shedMiddle`'s final fallback
      (`ansi.Truncate(join(nil, false), width, "…")`) passes `shed=false` to `join`,
      so the shed marker it would otherwise append is skipped in favour of
      `ansi.Truncate`'s own "…" — only reachable once lead+trail alone still overflow
      width, which needs a terminal too narrow for any footer to be useful at all.
      Not changed (reviewer called it realistically unreachable); a comment now says
      why the two truncation paths differ instead of leaving it looking like an
      oversight.
- [x] 12.7 Non-blocking, fixed: round 11's `globalShedOrder` ranked "tab" as its
      second-least-useful entry, so `⇥/[] tab` — switching which of the seven tabs is
      active, not a rare action — was among the first things dropped at 80 columns,
      alongside genuinely rare entries like jump-by-number and pane-switching. Moved
      "tab" from 2nd-least-useful to 3rd-most-protected, so it now survives alongside
      movement and quit at the default terminal width. Verified manually at 80/100/120
      columns before folding the change into this round; no dedicated test added since
      `TestGlobalFooterShedsByUsefulnessNotDeclarationOrder` already covers this
      class and does not need new assertions to keep passing.
- [x] 12.8 Verified the full round: `go build ./...` and
      `go test ./internal/ui/tui/...` both green after every fix in this section,
      including the pre-existing `TestGlobalFooterShedsByUsefulnessNotDeclarationOrder`.

## 13. Review fixes, round 13

- [x] 13.1 SPEC VIOLATION: `globalFooter` ranked focus-split parts by their own display
      label ("item", "goto", "copy" — minted by `focusSplit`, never declared in
      `globalShedOrder`), so `shedPriority` returned its not-found fallback
      (`len(globalShedOrder)`, the highest number) for every one of them — sorting
      them LAST, i.e. kept longest, ahead of every named entry. At 80 columns focused,
      `G move/top/bot` was shed while `y copy` survived; at 60, `q quit` was shed while
      `item`/`goto`/`copy` all survived — the reverse of "basic movement and quitting
      SHALL be among the last entries shed", which the requirement does not scope to
      the unfocused case. Fixed by adding `globalEntry.rank`, set to the PARENT
      binding's own label (`b.label(m)`, e.g. "move/top/bot") rather than the
      focus-remapped display label — `shedMiddle` now sorts by `rank`, so a
      focus-split part always sheds exactly when its parent binding would.
      `TestGlobalFooterShedsByUsefulnessUnderFocusToo` pins it; confirmed it catches
      the regression by reverting `rank` to the display label and watching the test
      fail before restoring the fix. The second-order symptom the reviewer flagged
      (the surviving middle entries duplicating `contextFooter`'s row while
      global-only entries like `tab` vanished) needed no separate fix — it was this
      same ranking bug, and resolves once the ranking is correct.
- [x] 13.2 Non-blocking, fixed: `shedPriority`'s comment claimed an unlisted entry "is
      never shed (kept even after everything named is gone)", but `shedMiddle`'s loop
      iterates every entry in `byPriority`, unlisted ones included at the tail — they
      are shed LAST, not never. Reworded to say "shed last (shed last, not skipped)"
      rather than "never shed".
- [x] 13.3 Non-blocking, fixed: two comments described the reference modal's own
      section order backwards. `keyHelp`'s doc comment
      (keys.go:28, the constant this whole feature adds) and `onkey.go`'s `case
      keyHelp` comment both said "global, then the current tab's" — but `helpLines`
      renders the current tab first, GLOBAL after, which is what the spec requires
      ("The current tab's own section SHALL lead the modal, GLOBAL after") and what
      `helpLines`' own doc comment already said correctly. On a change whose entire
      point is that a binding and its help cannot drift apart, fixed the wording to
      match the code and the spec, not the other way around.
- [x] 13.4 Verified the full round: `go build ./...`, `go test ./internal/ui/tui/...`,
      `gofmt -l`, and `brokkr lint comment-length` on the touched files all clean.
      The two smaller notes (`scopeName`'s redundant bool parameter, `shedMiddle`'s
      dropped-map keyed by label) were left as-is per the reviewer's own "no action
      required if you disagree" — both describe a real but currently harmless
      redundancy, not a bug.
