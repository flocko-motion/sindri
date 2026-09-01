# Tasks

## 1. The observer announces

- [ ] 1.1 `record()` compares the settled reading it is about to store against the last ANNOUNCED
      one and announces the difference, carrying both. Settled, not raw: `strikes` already holds
      `up` until `downStrikes`, and `stillSince` already dampens stillness — a subscriber sees what
      the hub stands behind.
- [ ] 1.2 `recordFill` too, or a fill crossing a threshold is invisible to a subscriber while the
      board shows it. Decide whether fill changes announce at all, or only on a threshold crossing,
      and say which — a per-sample fill announcement is a per-beat announcement wearing a hat.
- [ ] 1.3 Announcing must not block or fail the sweep. Dispatch off the observer's own goroutine,
      drop or coalesce when a subscriber is behind, and record the drop. A test that a hanging
      subscriber leaves the beat running.
- [ ] 1.4 `statuswatch` already diffs and only logs. Either it becomes the announcer, or it becomes
      a subscriber and stops diffing separately — two components computing "did this change" is how
      they come to disagree. Say which and why.

## 2. The pollers become subscribers

- [ ] 2.1 `stallwatch`'s ticker: the stall question is answered by the reading that saw the agent
      stop. Subscribe; keep an interval only as a declared backstop, with its purpose on it.
- [ ] 2.2 Mail re-announcement: `ReannounceAfter`'s five minutes exists because nothing said "this
      agent is reachable now". Announce on the change instead. The timed re-announcement was itself
      a fix for a one-shot announcement that got lost — so the subscriber must be at-least-once,
      and the backstop stays until that is proven.
- [ ] 2.3 Delivery reads the readiness the observer published rather than sampling its own — the
      race behind "pushed: true" over a pane that never showed the text.
- [ ] 2.4 Every interval deleted here gets a test for the behaviour it used to provide. An event
      that never fires is a quieter failure than a timer that fires too slowly, which is why this
      is not a straight deletion.

## 3. The seam holds

- [ ] 3.1 The subscription is on the `Harness` port, so the orchestrator receives observations from
      the same place it asks for them and no caller reaches into the watchdog.
- [ ] 3.2 An arch test that only the observer announces — the same shape as the guard that already
      holds the runtime-polling monopoly, and for the same reason.
