# Tasks

Note on names: `ClearContext` has since become `SetClearArmed` and its `fireClear`, which arms a
clear and lands it at the agent's next leaf boundary (sd-4a67ad). Everything below still describes
what happens, at the moment the clear actually fires.

## 1. Invalidate at the action

- [x] 1.1 `ForgetContext` drops one agent's cached reading.
- [x] 1.2 `ClearContext` calls it before the kickoff — the kickoff is what makes the agent ask, and
      the answer is computed from that reading.
- [x] 1.3 Leave `contextTTL` alone: it is right for every other reader.

## 2. The same line fixes the board

- [x] 2.1 `ContextFull` reads the same memo, so a cleared agent stops showing as full at once.

## 3. Do not chase an agent that was told to wait

- [x] 3.1 Exempt retired-by-a-human and retired-by-context from the idle nudge.
- [x] 3.2 The idle nudge ONLY. My first attempt sat above the api-error branch and swallowed the
      retry; the test caught it, and the ordering is now pinned.

## 4. Pin it

- [x] 4.1 The DIRECTIVE, not the memo entry: full reading turns the agent away, corrected reading
      hands it the waiting task.
- [x] 4.2 The board agrees at the same moment.
- [x] 4.3 The nudge control FIRST — a genuinely quiet agent holding work is still nudged — so an
      exemption that simply never nudged could not pass.
- [x] 4.4 Retired and full both exempt; a cut-off turn still retried.
- [x] 4.5 The CLEAR ITSELF drops the reading — the gap this change reported rather than implied.
      Every other case invalidates the memo directly, so removing the call from the clear passed.
      Driving it needs a running pod and a transcript, and both are wireable after all: the package
      already fakes the container runtime (`tell_signedout_test.go`), `agentport.Use` supplies the
      real transcript reader, and `SINDRI_HOME` points the agent's home at the test's own directory.
      The case changes the transcript between two readings, as a real clear does, and fails when the
      call is removed.
