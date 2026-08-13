# Tasks

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
- [ ] 4.5 NOT PINNED: that `ClearContext` itself performs the invalidation. Removing the call passes,
      since the tests invalidate directly. Driving `ClearContext` needs `container.Running` and a
      tmux pane read, neither behind the module's `Deps` — disproportionate to fake for one call.
      Reported rather than implied covered.
