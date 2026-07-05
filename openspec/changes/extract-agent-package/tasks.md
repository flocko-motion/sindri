## 1. Create the `internal/agent` package

- [ ] 1.1 Add `internal/agent/agent.go` with the four-field header and move `AgentView` + `ClientView` there (keep the exact JSON tags, incl. `container`, `clients`).
- [ ] 1.2 Move `Container(root, name string) string` into `internal/agent` (from `internal/hub`).
- [ ] 1.3 Move `parseClients` and `FormatClients` into `internal/agent`; export `parseClients` only if `hub` needs it, else keep it unexported and call it via a small exported entry the hub uses.
- [ ] 1.4 Move status collapsing into a pure `agent` function that takes primitives (running bool, workflow phase, …) instead of a `*Hub` receiver.
- [ ] 1.5 Confirm `internal/agent` imports no `hub`, `store`, or `pod` package (`go list -deps` / build check).

## 2. Rewire the hub

- [ ] 2.1 Update `internal/hub/state.go`: `BoardState.Agents` becomes `[]agent.AgentView`; `State` constructs `agent.AgentView` and calls `agent`'s status/clients helpers.
- [ ] 2.2 Update `hub.Clients`/`clientsCtx` to return `[]agent.ClientView` (still doing the `pod.ExecContext` probe here).
- [ ] 2.3 Replace in-hub uses of `hub.Container(...)` with `agent.Container(...)`.
- [ ] 2.4 Update `internal/hub/server.go` `/agent/clients` handler for the new return type.

## 3. Rewire clients and UIs

- [ ] 3.1 `internal/client`: `Clients` returns `[]agent.ClientView`.
- [ ] 3.2 `cmd/sindri`: update the `backend` interface (`Clients`), `agent.go`, and `attach.go` to reference `agent.AgentView`/`agent.ClientView`/`agent.Container` and `agent.FormatClients`.
- [ ] 3.3 `internal/tui`: update `tab_agents.go`, `messages.go`, `refresh.go`, `tui.go` to reference the `agent` package; drop `hub` imports that were only for these types.

## 4. Verify

- [ ] 4.1 `make verify` (build + test + lint) is green.
- [ ] 4.2 Grep confirms no UI file imports `hub` solely for a domain type; `internal/agent` has no service/adapter imports.
- [ ] 4.3 Drive it end-to-end: `sindri agent list`/`info` and the TUI still show repo, `👁N`, and client detail identically — no wire/behavior change.
