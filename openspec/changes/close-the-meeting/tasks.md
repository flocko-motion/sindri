# Tasks

## 1. Close the meeting

- [x] 1.1 `Service.Close` empties the roster, tells each member, and keeps the transcript.
- [x] 1.2 Closing an empty room does nothing — no notice, no transcript line.
- [x] 1.3 `C` on the Chat tab, confirmed; `sindri meeting close`, immediate.

## 2. Auto-close after an hour of silence

- [x] 2.1 `meetingIdle` is an hour, with the reasoning stated as `refInterval`'s is.
- [x] 2.2 Measured from the last MESSAGE, and why, since the two rules differ for a reader.
- [x] 2.3 Checked on the hub's existing slow loop rather than a ticker of its own.
- [x] 2.4 The clock is read at the edge, so the rule is exercisable without waiting an hour.

## 3. Quiet, and traceable

- [x] 3.1 The automatic close delivers when ready; the deliberate one may interrupt.
- [x] 3.2 The reason is recorded in the transcript, and per agent as membership changes are.

## 4. Pin it

- [x] 4.1 A closed room is empty, its members told, its transcript intact and explaining why.
- [x] 4.2 An empty room closes to nothing.
- [x] 4.3 A room spoken in within the hour stays open; an hour later it does not.
- [x] 4.4 The automatic notice takes the when-ready path, not the interrupting one.

## 6. Keep the files within their limits

- [x] 6.1 The meeting's client methods move to `internal/client/chat.go` with their own header —
      `client.go` went over 700 lines once this branch met what arrived beside it.
