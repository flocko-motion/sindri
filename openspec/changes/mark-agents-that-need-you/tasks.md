# Tasks

## 1. Write the rule once

- [x] 1.1 `api.AgentNeedsUser` states the rule — a state that resolves only if a human acts — and
      names the four statuses that satisfy it today.
- [x] 1.2 The three idle buckets are written beside it, so the marker and the idle observer cannot
      disagree the first time a status is added.
- [x] 1.3 The status words each get a constant, so the rule is not four literals.

## 2. Carry it on the section model, not in a view

- [x] 2.1 `api.Section` gains `Attention`; `hub/commands` gains the recipe per section, nil where a
      section holds nothing a human waits on.
- [x] 2.2 `Resolved` sends both numbers, and the board carries the resolved sections so a front-end
      renders rather than derives.
- [x] 2.3 The TUI draws every handle in one loop over the sections, and the Tasks special case goes.

## 3. Same states, same visibility, in the CLI

- [x] 3.1 `sindri agent list` marks each row that waits on the user.
- [x] 3.2 It closes with a line naming them and what clears each, since a status column is skimmed.

## 4. Pin it

- [x] 4.1 The rule: every state that counts, every state that must not — idle and retired first.
- [x] 4.2 The counts: both sections against a real board, and a nil recipe resolving to zero.
- [x] 4.3 The board carries its sections, since a front-end that finds none marks nothing.
- [x] 4.4 The handles render a marker per section, and none when nothing waits.
