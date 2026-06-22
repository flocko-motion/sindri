# Add a host (human) approve for PRs, with immediate merge feedback

## Why

The human is meant to hold the final say over what lands, but today the host
exposes only half of a verdict. A human can **reject** a PR from the host
(`RejectPR`, the `R` action in the PRs tab) — but there is no human **approve**.
Only the reviewer agent's `approve` verb sets a PR to `approved`, and `Merge`
refuses anything that is not `approved`:

- `internal/hub/workflow_pr.go` — `cmdApprove` is reviewer-only; `Merge` requires
  `pr.Status == "approved"`.

So when no reviewer agent is running (or the user simply wants to sign off
themselves), a human can reject a PR but can never move it to `approved`, and
therefore can never merge it. The one hard human gate is unreachable from the
positive side. The reviewer agent is effectively mandatory, which contradicts
"merge is human-only, the final say."

Separately — a small UX gap surfaced alongside this — `[m]erge` in the PRs tab
runs synchronously with no immediate feedback: the row sits unchanged until the
hub's board event lands, so the TUI looks momentarily hung.

## What Changes

- **Human approve on the host.** Add `sindri pr approve <id>` (and the matching
  hub + client path), the positive counterpart of the existing human reject. It
  marks the PR `approved` and satisfies its review gates, so the user can then
  merge — with or without a reviewer agent in the loop. Only an open PR (one
  awaiting a verdict) can be approved.
- **Approve in the PRs tab.** The TUI PRs tab gains an approve action alongside
  reject and merge, distinct from the existing `A` "request an agentic review".
- **Approval is not the reviewer's exclusive power.** The role model is updated:
  a worker's PR is reviewed by the reviewer agent **or** approved/rejected by a
  human on the host. The "no agent approves or merges its OWN work" rule and
  human-only merge are unchanged.
- **Immediate merge feedback.** When the user triggers a merge, the PRs tab shows
  a transient "merging" indicator on the row at once, replaced by "merged" when
  the hub confirms (or cleared on error), instead of appearing to hang.

## Capabilities

### Modified Capabilities

- `03-gh-local`: role-scoped commands — the host also exposes a human approve, so
  a PR can reach `approved` without a reviewer agent; merge stays human-only.
- `05-workflow`: plan/build/review separation — a worker's PR may be approved or
  rejected by the reviewer agent **or** by a human on the host; no agent approves
  its own work and merge remains human-only.
- `view-tui`: the dashboard control surface — the PRs tab offers approve (next to
  reject and merge), and a non-instant action (merge) gives immediate feedback.

## Impact

- **Source of truth:** `internal/hub/workflow_pr.go` (new `ApprovePR`, mirroring
  `RejectPR`), `internal/client/client.go` (client method), `cmd/sindri`
  (`pr approve` verb), `internal/tui/tab_prs.go` + `internal/tui/tui.go` (approve
  action + footer hint + transient merge feedback).
- No change to the merge gate itself: merge still requires `approved`; this only
  adds a second, human, way to reach `approved`.
