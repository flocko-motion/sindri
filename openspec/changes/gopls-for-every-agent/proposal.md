# Every agent gets the Go tools, and the shim explains a tree it cannot serve

## Why

Agents asked permission to install gopls, which the image already ships. The binary was never the
problem — `/usr/local/bin/gopls` v0.23.0, installed by the Dockerfile. The wiring was.

`mcpServers()` declared the server only when a `go.mod` sat at the WORKSPACE ROOT, tested once while
the pod's Claude home was prepared. That is two failures in one condition:

- A module anywhere but the root — a subdirectory, several modules under a `go.work`, a mixed
  language tree — is a Go project gopls serves perfectly well, and the root `stat` called it none.
- It is evaluated at one moment. An agent launched before its worktree is populated fails the check,
  and nothing revisits it: the config is per agent and persists across launches, so one badly timed
  launch leaves that agent without Go tooling indefinitely.

An agent that finds gopls unreachable through the tools it was told about does the sensible thing
and asks to install it.

I wrote that condition. It looked like restraint — do not start a language server for a language the
project does not use — and it was really a guess about the tree, made in the wrong place and cached
for the life of the agent.

## What changes

- The server is declared for every pod, unconditionally. `mcpServers()` takes no workspace at all
  now, because there is nothing left to decide from: the pod mounts the tree at `/workspace`
  whatever the host path was.
- The shim locates the module: `go.work` or `go.mod` at the root, else the shallowest `go.mod`
  within three directories, skipping `.git`, `vendor`, `node_modules` and friends. Breadth-first, so
  a repo whose root module contains a nested one is served from the root.
- Where there is genuinely nothing to serve, it says so — naming the directory searched and the
  depth, and stating that nothing was examined. That is the counterpart obligation: with no
  condition at launch, this is the only thing that can tell a non-Go tree from a broken one, and an
  empty result would read as a fact about the code. It already did exactly this for a refused Go
  toolchain; this extends the same reasoning to a tree with no module.
- `brokkr gopls-mcp` is no longer hidden.

## The help question, answered rather than assumed

It was deliberate: `Hidden: true`, with a comment saying it speaks JSON-RPC and is for Claude to
spawn. That reasoning was sound when the server was declared for some pods; it is not now that every
agent depends on it. An agent whose Go tools are unavailable needs to find the command that would
tell it why. It is listed, and its one-line help says who spawns it and what happens if you run it
by hand.

## The cost, stated so nobody re-litigates it

A non-Go pod now carries a declared MCP server it will not use. It is a stdio shim, started on
demand, so the cost is a process that answers "no Go module here" if anything ever asks — and
nothing does, in a tree with no Go. That is the trade: a wasted process description against an agent
silently losing its tooling.

## Impact

- Specs: `agent-runtime` gains the requirement.
- Code: `internal/adapter/agent/claude/home.go` (the guards), `internal/brokkr/goplsmcp` (module
  location and the refusal), `cmd/brokkr/main.go` (the listing).
- Replaces the test asserting a non-Go workspace gets nothing. It encoded the rule this reverses,
  and says so.
