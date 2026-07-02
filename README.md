# Sindri

A sandboxed AI-agent orchestrator. You hand work to agents that run inside
containers; they write code and open pull requests; **you approve the merge** —
the one hard gate. A single per-repo **hub** owns all state and mediates
everything, so the CLI, the TUI, and every agent are just thin clients of it.

This README is about *using* sindri. For the internal design, see `openspec/`.

---

## Install

Download the latest release and install it — a one-time step (after that,
`sindri upgrade` and the daily check keep you current). Grab the `.deb` from the
[releases page](https://github.com/flocko-motion/sindri/releases/latest), or pull
the latest from the command line:

```bash
url=$(curl -fsSL https://api.github.com/repos/flocko-motion/sindri/releases/latest | grep -o 'https://[^"]*_amd64\.deb' | head -1)
curl -fsSL "$url" -o /tmp/sindri.deb && sudo apt install -y /tmp/sindri.deb && rm -f /tmp/sindri.deb
```

That's it. The package bundles everything sindri ships — the `sindri` CLI/TUI, the
agent browser `sindri-worker` (it runs as `sindri` inside a pod), the `brokkr` toolbelt (code map + linters),
and the `td` task backend (plus `yq`) — and pulls in the only system tools it
needs, **git** and **podman**.

The one thing you bring yourself: **Claude credentials** at `~/.claude` (sindri
seeds them into the agent pods). The agent container image is built automatically
on first `sindri agent start` (needs network that once).

Then, in any repo, `sindri coauthor` gets you going in one command (see the Quick
start below) — it starts the per-repo hub for you, so you rarely launch one by hand.

**Optional extras** (sindri degrades gracefully without them, with a visible
note — never a hard failure): `openspec` (`npm i -g @fission-ai/openspec`) for the
spec-driven workflow, and the Go toolchain for the `deadcode` linter.

### Updating

Sindri checks for a newer release once a day (and on demand via **`sindri
upgrade`**); when there is one it points you at **`sindri-do-upgrade`** — a
one-shot script it drops in `~/.local/bin` that fetches and installs the latest
`.deb`. (The check can't replace the running binary itself, so the install is a
separate script.)

---

## Quick start

The simplest way in: **pair with an agent in your repo** — like running Claude
yourself, but sandboxed in a container. One command, in any git repo:

```bash
sindri coauthor
```

It starts everything it needs (the per-repo hub, a sandboxed pod, an agent
auto-named after a Norse dwarf) and drops you into its terminal. The coauthor
works on your **actual checkout** — you share the same files and drive it
directly, with no task queue and no PR gate. Detach with your tmux prefix then
`d` to leave it running; `sindri coauthor` again reattaches. That's the whole
thing.

### Advanced: managed workers and the task loop

When you'd rather hand off work than pair: put **one worker** on a task and merge
what it produces — you're the reviewer, no second agent needed.

```bash
sindri hub start --bg                       # start the per-repo hub in the background
                                            # (foreground: `sindri hub start`; see also `sindri hub list` / `sindri hub stop`)

sindri task new "Add a /healthz endpoint"   # describe a task
sindri agent new                            # create a worker — auto-named (e.g. dvalin)
sindri agent start dvalin                   # start it: it claims the task and starts coding

sindri tui                                  # watch the board live
```

Use the name `agent new` printed (or `sindri agent list` to see it). The worker
writes code in its own sandbox and opens a PR; review and land it:

```bash
sindri pr list
sindri pr approve pr-td-abc123              # sign off (you're the reviewer)
sindri pr merge   pr-td-abc123              # the one hard gate — human only
```

Everything beyond this — a reviewer agent, a planner, the collaborative "work a
whole feature" workflow — is opt-in, for when you want more. And a running hub
serves any number of agents at once: `sindri tui` shows the whole board, and you
can add workers alongside a coauthor.

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
│            │ pod: dvalin   │   │ pod: reviewer │   …            │
│            │  Claude+tmux  │   │  Claude+tmux  │                │
│            └───────────────┘   └───────────────┘                │
└──────────────────────────────────────────────────────────────┘
```

- **Single writer.** The hub is the only thing that touches td, git, and
  `.sindri/`. Every UI reads `GET /state` and live-updates over `GET /events`.
- **Identity is the socket.** Each pod mounts one socket; the hub knows who's
  calling by which socket accepted the connection — no names on the wire.
- **The agent is a browser.** Inside a pod, the agent's command `sindri` has *no
  built-in commands*: run it with no arguments and the hub tells it the one thing
  to do next, filtered by role and state. A command it can't run is invisible.
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
3.  sindri submit "…"                 → registers a merge-intent; returns at once
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

- The agent works a subtask, runs **`sindri checkpoint "…"`** → commits to
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

## Dev tooling — `brokkr`

`brokkr` is sindri's toolbelt: a separate, hub-less binary with the generic Go
tools. Works on any repo, no orchestration involved.

### Linters — `brokkr lint`

```bash
brokkr lint                # run them all (gates submit/CI); exits non-zero on any violation
brokkr lint deadcode       # unreachable functions (RTA); tests are live code
brokkr lint loc            # files over the 700-line limit
brokkr lint comments       # canonical file headers + documented exported funcs/types
brokkr lint openspec       # validate openspec specs (skips if unused/uninstalled)
```

- `brokkr lint` (no arg) runs every linter with a summary; `brokkr lint <name>`
  runs just one.
- Exits non-zero on any violation and turns a panic into a marked failure, so it
  gates CI. Add **`--tail N`** (on any brokkr command) to buffer the output, print
  only its last N lines, and end with a **`=== exit: <code> ===`** marker — the
  exit status inline, so you (or an agent) never append `echo "$?"`.
- **`deadcode`** always analyses test packages (tests are live code), and skips
  with a note if the `go` toolchain isn't on PATH.
- **`comments`** enforces the project convention: every non-test `.go` file opens
  with a four-field header (`package` / `type` / `job` / `limits`, the block
  `brokkr map` reads), and every exported function and type has a doc comment. On
  a violation it prints the convention with a short example. Each header field's
  content is also length-bounded (so the map stays compact); extra free-form
  comments in the header don't count.

### Codebase map — `brokkr map`

A structured overview to navigate by, instead of reading whole files: per file,
the header plus each type/func with its doc and signature (bodies omitted).

```bash
brokkr map                              # whole tree
brokkr map internal/hub internal/tui    # several paths at once
brokkr map --grep "func Merge"          # only decls whose source matches
brokkr map internal/tui --file tab_prs  # only files whose path matches
brokkr map --depth 1                    # bound how deep it descends
brokkr map --full                       # don't reduce, however long
```

If the full map runs past a line budget (default 1000, `--max`), it reduces to
per-file headers only and tells you so — narrow the scope or pass `--full`.

---

## Command reference

Orchestration is `sindri <category> <action>`; the toolbelt is the separate
`brokkr` binary.

| Category | Actions |
|---|---|
| `agent` | `list` · `new [name] [--role worker\|reviewer\|planner]` · `start <name>` · `stop <name>` · `delete <name>` · `tell <name> "msg"` · `attach <name>` · `info <name>` · `pane <name>` |
| `task` | `list` · `new <title> [-t -p -d --labels --parent]` · `info <id>` · `edit <id>` · `priority <id> <P0..P4>` · `approve <id>` · `reject <id> "why"` · `unassign <id>` |
| `pr` | `list` · `info <id>` · `lint <id>` · `verify <id>` · `review <id> "…"` · `approve <id>` · `reject <id> "…"` · `milestone <agent>` · `merge <id>` |
| `brokkr` | `map [paths…] [--grep --file --depth]` · `lint [deadcode\|loc\|comments\|openspec]` (none = all) |

Inside a pod the agent talks to the hub through a single command, **`sindri`**
(the browser binary, presented under that name in the isolated container) — run
with no args to get its next directive, or a verb the hub currently offers it:
workers get `next`/`submit`/`checkpoint`/`show`/`lint`; reviewers
`approve`/`reject`/`review`; planners `task`/`create-task`/`openspec`/`state`; all
get `status`/`log`/`prs`.

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
cmd/sindri/         host CLI (agent/task/pr + hub + tui)
cmd/sindri-worker/  the agent's thin browser (no command tree; `sindri` in a pod)
cmd/brokkr/         the toolbelt: code map + linters (no orchestration)
internal/hub/       the hub: service, SQLite store, command registry, workflows
internal/client/    thin hub client (CLI + TUI share it)
internal/adapter/   one package per external tool: git, pod (podman), tmux, td, spec
internal/tui/       lean Bubble Tea dashboard (a hub client)
internal/lint/      the linters; internal/codemap/ the code map
container/          the agent image (Dockerfile) + tmux entrypoint
openspec/           the spec-driven design (specs + changes)
```

---

## Building from source

For hacking on sindri (end users just install the `.deb`). Needs Go, plus `td`
and `yq` on `PATH` (they get bundled into the build).

```bash
make           # (or make help) list all targets
make install   # build sindri + sindri-worker + brokkr, install to ~/.local/bin
make all       # + build the agent image too (needs podman)
make verify    # run the linters (the gate; release runs this first)
make check     # build + test + lint — the quality gate
make deb       # build the .deb into bin/
make release <major|minor|patch>   # lint, then release: push, open+merge a PR (gh), tag the merged default branch, return you to your branch (breaking|feature|fix aliases too)
```

---

## Acknowledgments

The Sindri TUI began as a fork of [sidecar](https://github.com/marcus/sidecar)
by Marcus; the current dashboard is a lean rewrite against the hub.
