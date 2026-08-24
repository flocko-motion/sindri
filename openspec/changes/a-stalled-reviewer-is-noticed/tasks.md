# Tasks

## 1. Count the reviewer

- [x] 1.1 `Stalled()` counts `"reviewing"` alongside `"working"`, and the comment says why reviewing
      is not a waiting phase while `submitted` and `gating` are.

## 2. Give the nudge something to name

- [x] 2.1 `NudgeStalled` falls back to `ps.ReviewingPR(name)` when Task and Container are both empty.
- [x] 2.2 A store fault on that read sends nothing, and no live review row sends nothing — neither
      nudges with a blank or an invented id.

## 3. Pin both halves

- [x] 3.1 The rule table gains a reviewer holding a PR (stalled), one under the dwell (not), and one
      blocked on the user (not).
- [x] 3.2 A nudge test drives the whole path from an assigned review row: the message names the PR and
      the log records it. It fails with either half of the fix removed, which is the claim that
      neither alone is worth landing.
- [x] 3.3 A board test asserts the row reads `stalled` and names the PR, since `stalledFor` is what
      makes the word and the prod one observation.
