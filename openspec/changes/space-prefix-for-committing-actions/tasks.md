# Tasks

## 1. The prefix

- [x] 1.1 Space opens the menu; a committing key is inert without it and acts through it.
- [x] 1.2 Escape, space again, or an unoffered letter closes it having changed nothing.
- [x] 1.3 Declared in the keymap like any other binding, so the footer generates from it.

## 2. Generated, not a second list

- [x] 2.1 `binding` gains `commits`; the menu, the footer and the dispatch gate all read it.
- [x] 2.2 `binding` gains `when`, and the menu offers only what applies to the selected row.
- [x] 2.3 Inertness ignores `when`: a committing key is never live bare, whatever is selected.

## 3. The convention

- [x] 3.1 Reworded to navigate/commit, with the exception clause removed rather than reworded.
- [x] 3.2 Lowercase navigation untouched — e, i, t, c, o, a stay direct.

## 4. The footer

- [x] 4.1 Navigation plus one entry, named readably ("space"), which is the room the movement keys
      need.
- [x] 4.2 A row with nothing to offer says so, rather than leading to an empty box.

## 5. The pinned tests

- [x] 5.1 Merge: still M, declared committing, offered by the menu on an approved PR only.
- [x] 5.2 The PR verdicts move to the menu; the editor stays in the footer.
- [x] 5.3 The lowercase allowlist stays, with a structural test beside it that needs no list.
- [x] 5.4 Every test that pressed a committing key presses the prefix first — the gesture changed,
      the actions did not.
