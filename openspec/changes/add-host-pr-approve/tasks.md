# Tasks

## 1. Human approve on the host

- [x] 1.1 Add `ApprovePR(prID)` to the hub in `internal/hub/workflow_pr.go`,
      mirroring `RejectPR`: open-only guard, set status `approved`, log it. (The
      merge gate is simply status == approved, so there's no separate gate to
      satisfy.)
- [x] 1.2 Expose `sindri pr approve <id>` on the host CLI (`cmd/sindri`), next to
      `pr merge` / `pr reject`, calling the new hub method.
- [x] 1.3 Add a client method `ApprovePR` in `internal/client/client.go`
      (+ `POST /pr/approve` route) so the TUI and CLI share one path.
- [x] 1.4 Add a human-approve action to the TUI PRs tab (`internal/tui/tab_prs.go`,
      `internal/tui/tui.go`) on its own key, distinct from the existing `A`
      "request an agentic review"; update the PRs footer hint to list it.

## 2. Immediate merge feedback

- [ ] 2.1 In the PRs tab, when the user triggers a merge, optimistically show a
      transient "merging" status on the selected row before the hub's board event
      lands.
- [ ] 2.2 Replace the transient state with the real status when the merged board
      event arrives, and clear it if the merge returns an error.

## 3. Verify

- [ ] 3.1 `openspec validate --all` passes (run by the submit lint gate).
- [ ] 3.2 Manual: with no reviewer agent running, approve a PR then merge it from
      both `sindri pr approve`/`sindri pr merge` and the TUI PRs tab; confirm a
      non-open PR cannot be approved and that merge shows immediate feedback.
