# Tasks

## 1. Refuse rather than silently rebase

- [x] 1.1 `refuseIfBehind` counts the branch against its base and refuses, recording no PR and
      leaving the agent's phase untouched.
- [x] 1.2 Place it BEFORE the gate: a gate run on a tree that must rebase is wasted, and a rebase
      after it would attach a verdict taken against the old base.
- [x] 1.3 A count that fails is not a refusal — evidence absent is not evidence against.

## 2. Say enough to act on

- [x] 2.1 Name the distance, the base, and both commands.
- [x] 2.2 List what arrived, so the agent can judge whether its work still holds.

## 3. Both paths that create a PR from an agent's own branch

- [x] 3.1 `CmdSubmit`.
- [x] 3.2 `CmdOpenspec`, the same hole, and planners have `rebase` too.
- [x] 3.3 Leave `contribute` alone: it already rebases and routes conflicts to `resolve`.

## 4. Pin it

- [x] 4.1 Refused when behind: no PR, phase unchanged, and the message carries count, base, commands
      and the incoming commits.
- [x] 4.2 A current branch still submits and records its base.
- [x] 4.3 The path the refusal points at ends in a PR: rebase, then submit.
- [x] 4.4 A refused submit commits nothing, so a retry is the same submission and not one layered on
      the last.
- [x] 4.5 Mutation-checked compilably: never refusing fails four tests, and moving the guard after
      the gate fails the no-commit test — which is what makes the ordering argument load-bearing
      rather than decorative.
