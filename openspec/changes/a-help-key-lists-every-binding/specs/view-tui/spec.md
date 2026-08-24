# view-tui — delta

## ADDED Requirements

### Requirement: A reference modal lists every hotkey

The TUI SHALL offer a reference modal, opened by `?`, listing every global binding and
every binding on the current tab. `?` SHALL be direct, not behind the committing-action
prefix, since opening it changes nothing.

The modal SHALL be generated from the same table that declares the bindings and renders
the footers, so a binding and its help cannot drift apart. A real dispatcher key the
footer deliberately omits — escape's clear-filters, enter's default expand-to-modal —
SHALL still carry a row in that table, marked so the footer keeps omitting it, so the
reference is not silently narrower than the dispatcher. Such a row SHALL be declared at
whatever scope the keymap already uses to keep one key's meaning from varying within a
single reading of the modal: global where the meaning does not vary by tab (escape),
per tab where it does (enter, whose default differs from Repos' and Chat's own). The
modal's own scrolling and closing SHALL NOT get a dedicated row: that chrome is common
to every such modal in the program and is already named on the modal's own hint line.
A GLOBAL row that happens to share a key with that chrome (move/top/bot, page, clear
filters, quit) still names its ordinary meaning — the reference does not hide or
re-describe it just because the reference modal also answers to the same key while it
is open.

Unlike the footer and the committing-action menu, the modal SHALL NOT hide a binding
whose condition does not currently hold. It SHALL list every binding for the tab
regardless, and SHALL name a binding's condition in words next to it, taken from the
keymap rather than written a second time. A binding that sits behind the committing
prefix SHALL be shown with the prefix folded into its key, so the reference does not
imply a bare keystroke reaches it.

The current tab's own section SHALL lead the modal, GLOBAL after: the reader is asking
what applies here, and the tab's own bindings SHALL NOT be pushed below the fold of a
terminal short enough to scroll. A binding declared at more than one scope SHALL appear
once, attributed to GLOBAL when one of the declarations is global, not to whichever
scope happens to render first.

While the detail or meta column has focus, a handful of keys mean something the tab's
ordinary bindings do not (moving among cross-references, opening one, yanking its
value). The modal SHALL relabel exactly those keys, from the same source the footer's
own focused hint uses, so the two never describe different worlds — and SHALL NOT
otherwise narrow what it lists: every other binding the tab or GLOBAL declares still
works under focus and SHALL still be shown. A row that mixes a focus-remapped key with
one that is not (moving among rows vs. jumping to the bottom of the list, say) SHALL be
split so each half keeps its own true meaning.

#### Scenario: The modal lists a binding the footer currently hides

- **WHEN** the selected task holds no agent, so the footer omits unassign
- **THEN** the `?` modal still lists unassign, with its condition in words

#### Scenario: The modal marks a committing binding

- **WHEN** a listed binding commits on the keystroke
- **THEN** the modal shows it reached through the prefix, not as a bare key

#### Scenario: The modal shows global and tab-local bindings together

- **WHEN** the user opens the modal from any tab
- **THEN** it lists the global bindings and that tab's own, in one reference

#### Scenario: A key with a per-tab meaning is not shown as one global reading

- **WHEN** enter's default action differs from what it does on Repos and Chat
- **THEN** each tab's reference names only what enter does there, never a single global
  line describing a meaning that is wrong on some tabs

#### Scenario: The current tab's bindings survive a short terminal

- **WHEN** the modal is opened on a terminal short enough that GLOBAL and the tab's
  section together do not fit the visible window
- **THEN** the tab's own bindings are what the reader sees without scrolling, not GLOBAL

#### Scenario: A binding declared at two scopes is listed once, under GLOBAL

- **WHEN** a binding also declared globally is redeclared on the current tab
- **THEN** the reference names it once, under GLOBAL, not under the tab section

#### Scenario: The reference matches the footer while the detail column has focus

- **WHEN** the user opens the modal with the detail or meta column focused
- **THEN** the tab section names what those keys currently do while focused, matching
  the footer's own hint, not their un-focused meanings

#### Scenario: A binding that still works under focus is still listed

- **WHEN** the detail or meta column has focus and the selected row offers a binding
  focus does not change (a `when`-gated one among them)
- **THEN** the modal still lists it, exactly as it would unfocused

#### Scenario: A mixed row splits into what changed and what did not

- **WHEN** a global row combines a key focus remaps with one it does not
- **THEN** the modal shows each half under its own true label, neither hidden nor
  misdescribed by the other

### Requirement: The committing-action prefix is advertised globally

The footer's global row SHALL name the committing-action prefix, since it works
identically on every tab; a tab-local row SHALL NOT repeat it. `?` SHALL lead the row
and the prefix entry SHALL trail it; both SHALL be the last two things dropped, since
each names where the rest of the row still reads in full — the reference for `?`, the
menu for the prefix. When the row cannot fit every entry at the terminal's width, it
SHALL shed whole entries from between them rather than truncate one mid-word, ending
with a mark that entries were dropped.

Which entries go first SHALL be decided by usefulness, not by the order the keymap
declares them in: basic movement and quitting the program SHALL be among the last
entries shed, and a rarer action (jumping to a tab by its header number, switching
which column has focus) SHALL be among the first — the row otherwise keeps what a
reader rarely needs and drops what they reach for constantly, at exactly the width
most terminals run.

#### Scenario: The prefix survives at an ordinary terminal width

- **WHEN** the user switches tabs at 80, 100 or 120 columns — widths the prefix
  previously rode on the tab-local row without trouble
- **THEN** the global row still names both `?` and the prefix entry

#### Scenario: A narrow terminal sheds whole entries from the middle

- **WHEN** the global row does not fit the terminal's width
- **THEN** entries between `?` and the prefix are dropped whole, never split mid-word,
  and the row still reads exactly the terminal's width

#### Scenario: Basic movement and quitting outlast the rarer entries

- **WHEN** the global row is narrowed to 80 columns, the default terminal width
- **THEN** it still names basic movement and quitting, while tab-jump-by-number and
  pane-switching — named the least useful — are among the first entries dropped

## MODIFIED Requirements

### Requirement: Agents and PRs tabs have a global/repo scope toggle

The Agents, PRs, Runs and Mail tabs SHALL each offer a scope toggle between `global` and
a narrow scope, defaulting to `repo`. In `global` the tab SHALL show the whole fleet
across every registered repo, each row repo-tagged. The active scope SHALL be shown in
the footer. This is a view filter only; it SHALL NOT change what data the hub holds, and
the Tasks tab SHALL remain always scoped to the active repo.

Runs and Mail SHALL toggle the same shared scope Agents and PRs do, not a scope of
their own: one setting, read by every scope-filtered tab, so a toggle made from one tab
is not silently undone by switching to another.

The narrow scope SHALL show the active repo's entries PLUS any entry from another repo
that is waiting on the user, on Agents and PRs. Agents and PRs are BACKGROUND work: they
progress while the user is looking somewhere else, which is exactly why something that
ends up waiting on them has to surface wherever they are. An approved pull request in
another repo is the one action only the user can take, and a scope that hid it left them
told that something needed them and shown a list where nothing did.

Mail carries the identical exception: the narrow scope SHALL keep every message
addressed to the user regardless of its repo, read or not, of which the mailbox's own
fleet-wide marker counts only the unread — so hiding any of them risks that marker
pointing at a row the list no longer shows.

Runs has no such exception — there is no fleet-wide "needs you" marker for a run — so
its narrow scope SHALL show only the active repo's entries, and its label SHALL say so
plainly, not reuse the wording Agents, PRs and Mail earn by keeping one.

Rows from another repo SHALL be grouped by repo rather than interleaved, so a foreign
row reads as what it is rather than as a filter that has stopped working.

The groups SHALL be LABELLED. When the list holds entries from another repo, they SHALL
sit under a heading naming them and counting them, above a heading naming the active
repo's, separated by a blank line. The foreign group SHALL come first: needing the user
is the only reason those entries are on screen, and beneath the local list they would be
found by scrolling again. The count in that heading and the attention badge on the tab
handle SHALL be one claim in two renderings — the badge says how many entries wait on
the user, the heading says which they are.

Neither heading SHALL appear when every entry is the active repo's. That is the ordinary
case, and a permanent heading taxes every ordinary glance to explain an occasional one.

A heading and its blank line SHALL NOT be selectable. The cursor walks the row list, so
a heading it could rest on is a selection with no entry behind it and a detail pane with
nothing to show.

Whether an entry is waiting on the user SHALL be decided by ONE predicate per kind, the
same one the attention marker and the row colour read, and the same one that decides
which group the entry belongs in. Visibility, colour and count derived separately drift,
and the drift is invisible because each looks plausible alone — a row shown here,
counted in the badge, and coloured as though nothing were owed.

The narrow scope's label SHALL say what it does. It admits entries from outside the
active repo on Agents, PRs and Mail, so a label naming the repo alone would be a filter
claiming to exclude what it plainly shows there — and, on Runs, where the label really
is the repo alone, it SHALL say exactly that rather than a wider claim it does not keep.

This SHALL NOT extend to the Tasks tab. A proposal awaiting a verdict is FOREGROUND
work: it was created in the repo the user is already in, and they will see it because
they are there.

#### Scenario: Default is global

- **WHEN** the user sets the Agents or PRs tab scope to `global`
- **THEN** it lists entries across all repos, each tagged with its repo

#### Scenario: Narrow to the active repo

- **WHEN** the user toggles the Agents tab scope to the narrow scope
- **THEN** it shows the active repo's agents, and the footer reflects the narrow scope

#### Scenario: The tab starts in repo scope, keeping anything that needs the user

- **WHEN** the Agents or PRs tab is first shown
- **THEN** it lists the active repo's entries, plus anything from any repo waiting on
  the user

#### Scenario: A pull request waiting on the user crosses repos

- **WHEN** the PRs tab is narrowed to the active repo and another repo holds a PR
  waiting on the user
- **THEN** that PR is listed, grouped under its own repo, and counted in the tab's badge

#### Scenario: The foreign group is labelled and counted

- **WHEN** the narrow scope holds entries from another repo that wait on the user
- **THEN** they appear first, under a heading naming them and stating how many, above a
  blank line and a heading naming the active repo's entries

#### Scenario: An ordinary list is unlabelled

- **WHEN** every entry the narrow scope shows belongs to the active repo
- **THEN** no heading and no blank line are shown, and the list reads as it did before

#### Scenario: The cursor steps over a heading

- **WHEN** the user moves the selection with `j`, `k`, `g`, `G` or a half-page key past a
  heading or its blank line
- **THEN** the cursor comes to rest on an entry, and the detail pane shows it

#### Scenario: The narrow scope still excludes

- **WHEN** another repo holds a PR that nothing is asked of the user for
- **THEN** it is not listed in the narrow scope

#### Scenario: The Tasks tab is unaffected

- **WHEN** another repo holds a task awaiting the user's verdict
- **THEN** the Tasks tab still shows only the active repo's tasks

#### Scenario: Narrow Runs to the active repo, plainly

- **WHEN** the user toggles the Runs tab scope to the narrow scope
- **THEN** it shows only the active repo's runs, labelled `repo`, with no needs-you
  exception

#### Scenario: The toggle also reaches Runs and Mail

- **WHEN** the user presses the scope key on the Runs or Mail tab
- **THEN** it toggles the same shared scope Agents and PRs use, narrowing or widening
  that tab's own rows accordingly

#### Scenario: Mail addressed to the user survives the narrow scope

- **WHEN** the Mail tab is toggled to the narrow scope
- **THEN** a message addressed to the user from another repo is still listed, matching
  the unread marker that counts it fleet-wide regardless of scope
