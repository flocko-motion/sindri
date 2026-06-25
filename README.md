# Sindri

A sandboxed AI-agent orchestrator. You hand work to agents that run inside
containers; they write code and open pull requests; **you approve the merge** —
the one hard gate. A single per-repo **hub** owns all state and mediates
everything, so the CLI, the TUI, and every agent are just thin clients of it.

This README is about *using* sindri. For the internal design, see `openspec/`.

---

## Install

Grab the `.deb` from the [latest release](https://github.com/flocko-motion/sindri/releases/latest)
and install it:

```bash
sudo apt install ./sindri_*.deb
```

That's it. The package bundles everything sindri ships — the `sindri` CLI/TUI, the
in-pod agent browser `sindri-worker`, and the `td` task backend (plus `yq`) — and
`apt` pulls in the only system tools it needs, **git** and **podman**.

The one thing you bring yourself: **Claude credentials** at `~/.claude` (sindri
seeds them into the agent pods). The agent container image is built automatically
on first `sindri agent start` (needs network that once).

Then, in any repo:

```bash
sindri hub &      # start the per-repo hub (everything is a client of it)
```

**Optional extras** (sindri degrades gracefully without them, with a visible
note — never a hard failure): `openspec` (`npm i -g @fission-ai/openspec`) for the
spec-driven workflow, and the Go toolchain for the `deadcode` linter.

### Updating

Sindri checks for a newer release once a day and, when there is one, tells you to
run **`sindri-update`** — a one-shot script it drops in `~/.local/bin` that
fetches and installs the latest `.deb`.

### From source (for hacking on sindri)

```bash
make all          # build the binaries + agent image, install to ~/.local/bin
make deb          # build the .deb into bin/  (bundles td + yq)
```

---

## Quick start

The happy path: put **one worker** on a task and merge what it produces — you're
the reviewer, no second agent needed. Run this in any git repo:

```bash
sindri hub &                                # start the per-repo hub

sindri task new "Add a /healthz endpoint"   # describe a task
sindri agent new                            # create a worker — auto-named (e.g. brokkr)
sindri agent start brokkr                   # start it: it claims the task and starts coding

sindri tui                                  # watch the board live
```

Use the name `agent new` printed (or `sindri agent list` to see it). The worker
writes code in its own sandbox and opens a PR; review and land it:

```bash
sindri pr list
sindri pr approve pr-td-abc123              # sign off (you're the reviewer)
sindri pr merge   pr-td-abc123              # the one hard gate — human only
```

That's the whole loop. Everything below — a reviewer agent, a planner, the
collaborative "work a whole feature" workflow — is opt-in, for when you want more.

---

## How it works (one picture)

```
┌──────────────────────────────────────────────────────────────┐
│ Host                                                           │
│   sindri CLI ─┐                          ┌─ sindri tui         │
│   (you)       ▼                          ▼  (live board)       │
│            ┌──────────────────────────────────┐                │
│            │  sindri hub   (single writer)     │                │
│            │  .sindri/hub.db  (SQLite)         │                │
│            │  td · git · openspec · podman     │                │
│            └───────┬───────────────┬──────────┘                │
│         per-agent  │ unix socket   │ tmux send-keys             │
│            ┌───────▼───────┐   ┌───▼───────────┐                │
│            │ pod: brokkr   │   │ pod: reviewer │   …            │
│            │  Claude+tmux  │   │  Claude+tmux  │                │
│            └───────────────┘   └───────────────┘                │
└──────────────────────────────────────────────────────────────┘
```

- **Single writer.** The hub is the only thing that touches td, git, and
  `.sindri/`. Every UI reads `GET /state` and live-updates over `GET /events`.
- **Identity is the socket.** Each pod mounts one socket; the hub knows who's
  calling by which socket accepted the connection — no names on the wire.
- **The agent is a browser.** Inside a pod, `sindri-worker` has *no built-in
  commands*: run it with no arguments and the hub tells it the one thing to do
  next, filtered by role and state. A command it can't run is invisible.
- **You hold the gate.** Merge is human-only.

---

## Roles

Three roles — start more agents as you need them (`sindri agent new --role <role>`,
auto-named after Norse dwarves):

- **worker** — builds: claims tasks, writes code, opens PRs. (The quick-start agent.)
- **reviewer** — reviews a worker's PR. Optional — you can approve/reject yourself
  on the host instead.
- **planner** — plans *with you*: reads the repo and specs, proposes tasks (you
  approve them), and ships specs as a PR. Never grabs backlog work.

You steer and manage any agent live — `agent tell <name> "…"`, `attach`, `stop`,
`delete`; `sindri tui` or `sindri agent list` show the board. (See the command
reference below.)

---

## The two workflows

Sindri supports two ways to get work done. They share the same machinery (one
branch + PR-as-merge-intent, git hub-side, human merge); they differ in how work
is grouped and when it's reviewed.

### 1. Structured — one task, one PR

The default. Good for independent, one-off tasks.

```
1.  worker claims the top task        → branch in /workspace
2.  edits /workspace                  → the hub commits
3.  sindri-worker submit "…"          → registers a merge-intent; returns at once
4.  …idle…                            → the agent waits (no polling)
5.  review                            → a reviewer agent, OR you on the host
6.  sindri pr approve <pr> && merge   → the human gate
7.  [hub] "merged — continue"         → the worker takes the next task
```

`submit` never blocks. Auto-assignment hands out **leaf tasks only** (a task with
no children).

### 2. Collaborative / bulk — a feature and its subtasks, one PR

For work that decomposes into subtasks. **Mark any parent task** and one agent
takes the whole thing on a single branch, landing subtasks back-to-back without a
review gate between them. The *same* flow covers two styles:

- **Bulk** — pre-fill the children, mark the parent, walk away.
- **Interactive** — feed subtasks live and ask for a PR at milestone moments.

```bash
# Build the feature: a parent (marked `collab`) with children.
sindri task new "Login feature" -t epic --labels collab          # → td-LOGIN
sindri task new "Form UI"   --parent td-LOGIN
sindri task new "Validation" --parent td-LOGIN
```

A free agent picks up the marked container automatically: it goes on a standing
branch named for the container and starts on the first child. Then:

- The agent works a subtask, runs **`sindri-worker checkpoint "…"`** → commits to
  the container branch, closes that child, and moves to the next — **no blocking**
  between subtasks.
- When you reach a milestone, **`sindri pr milestone <agent>`** captures the
  branch's current state as one PR and **blocks** the agent.
- You review it, then **`sindri pr approve pr-td-LOGIN`** and
  **`sindri pr merge pr-td-LOGIN`**. The merge lands, the branch is rebased onto
  the new base, and the agent **resumes the same feature** — the branch isn't
  retired.
- The agent is freed only when the container task itself is closed.

A milestone PR is the *one* deliberate pause in this workflow; everything else
streams. Reviewer opinions can be requested (`sindri pr review`) but are advisory
here — you own the merge.

---

## Reviewing & merging

```bash
sindri pr list                       # pending merge-intents
sindri pr info pr-td-abc123          # metadata + diff
sindri pr lint pr-td-abc123          # run the quality gate against the PR
sindri pr verify pr-td-abc123        # check it out into a workspace to run by hand

sindri pr review pr-td-abc123 "…"    # request an agentic review (assigns a reviewer)
sindri pr approve pr-td-abc123       # approve it yourself (no reviewer needed)
sindri pr reject  pr-td-abc123 "…"   # reject with feedback (routed to the worker)
sindri pr merge   pr-td-abc123       # the hard gate — human only, requires approved
```

A worker's PR reaches `approved` via a reviewer agent **or** your own
`pr approve`. Merge always requires `approved`, and only a human merges.

---

## Tasks

Tasks live in `td` (the source of truth), cached into the hub.

```bash
sindri task new "Fix the parser" -t bug -p P1      # type: bug|feature|task|epic|chore
sindri task new "Sub-thing" --parent td-abc123     # a child (subtask)
sindri task list
sindri task info td-abc123
sindri task edit td-abc123 --labels collab         # mark a parent for the collaborative flow
sindri task priority td-abc123 P0
```

A **planner** proposes tasks that you gate: a proposed task is *pending* until you
`sindri task approve <id>` (or `sindri task reject <id> "why"`); no worker can
claim it before then.

---

## Dev tooling

These work on any Go project, with or without a hub.

### Linters — `sindri lint`

```bash
sindri lint all            # run them all (gates submit/CI); ends with "=== EXIT N ==="
sindri lint deadcode       # unreachable functions (RTA); tests are live code
sindri lint loc            # files over the 700-line limit
sindri lint comments       # canonical file headers + documented exported funcs/types
sindri lint openspec       # validate openspec specs (skips if unused/uninstalled)
```

- Every subcommand ends with a loud **`=== EXIT N ===`** marker and turns a panic
  into a marked failure — so you (or an agent) never have to append `echo "$?"`.
- **`deadcode`** always analyses test packages (tests are live code), and skips
  with a note if the `go` toolchain isn't on PATH.
- **`comments`** enforces the project convention: every non-test `.go` file opens
  with a four-field header (`package` / `type` / `job` / `limits`, the block
  `code map` reads), and every exported function and type has a doc comment. On a
  violation it prints the convention with a short example.

### Codebase map — `sindri code map`

A structured overview to navigate by, instead of reading whole files: per file,
the header plus each type/func with its doc and signature (bodies omitted).

```bash
sindri code map                              # whole tree
sindri code map internal/hub internal/tui    # several paths at once
sindri code map --grep "func Merge"          # only decls whose source matches
sindri code map internal/tui --file tab_prs  # only files whose path matches
sindri code map --depth 1                    # bound how deep it descends
```

---

## Command reference

`sindri <category> <action>`. First-order: `hub`, `tui`, `lint`, `code`.

| Category | Actions |
|---|---|
| `agent` | `list` · `new <name> [--role worker\|reviewer\|planner]` · `start <name>` · `stop <name>` · `delete <name>` · `tell <name> "msg"` · `attach <name>` · `info <name>` · `pane <name>` |
| `task` | `list` · `new <title> [-t -p -d --labels --parent]` · `info <id>` · `edit <id>` · `priority <id> <P0..P4>` · `approve <id>` · `reject <id> "why"` · `unassign <id>` |
| `pr` | `list` · `info <id>` · `lint <id>` · `verify <id>` · `review <id> "…"` · `approve <id>` · `reject <id> "…"` · `milestone <agent>` · `merge <id>` |
| `code` | `map [paths…] [--grep --file --depth]` |
| `lint` | `all` · `deadcode` · `loc` · `comments` · `openspec` |

Inside a pod the agent uses the `sindri-worker` browser — run with no args to get
its next directive, or a verb the hub currently offers it: workers get
`next`/`submit`/`checkpoint`/`show`/`lint`; reviewers `approve`/`reject`/`review`;
planners `task`/`create-task`/`openspec`/`state`; all get `status`/`log`/`prs`.

---

## State & layout

| What | Where |
|---|---|
| Roster, workflow state, PRs, activity log | `.sindri/hub.db` (SQLite, gitignored) |
| Per-agent socket | `.sindri/sockets/<name>.sock` |
| Agent Claude home | `.sindri/claude/<name>/` |
| Code / commits | `.worktrees/<name>` (host) |
| Tasks (source of truth) | `td` (cached into `hub.db`) |

Throw a pod away freely; relaunch resumes from the activity log. Restart the hub
freely; nothing committed is lost.

```
cmd/sindri/         host CLI (agent/task/pr/code/lint + hub + tui)
cmd/sindri-worker/  the agent's thin browser (no command tree)
internal/hub/       the hub: service, SQLite store, command registry, workflows
internal/client/    thin hub client (CLI + TUI share it)
internal/adapter/   one package per external tool: git, pod (podman), tmux, td, spec
internal/tui/       lean Bubble Tea dashboard (a hub client)
internal/lint/      the linters; internal/codemap/ the code map
container/          the agent image (Dockerfile) + tmux entrypoint
openspec/           the spec-driven design (specs + changes)
```

---

## Acknowledgments

The Sindri TUI began as a fork of [sidecar](https://github.com/marcus/sidecar)
by Marcus; the current dashboard is a lean rewrite against the hub.
