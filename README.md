<p align="center">
  <img src="assets/web/emblem.png" alt="" width="160">
</p>

<p align="center">
  <img src="assets/web/wordmark.png" alt="Sindri" width="420">
</p>

A sandboxed AI-agent orchestrator. You hand work to agents that run inside
containers; they write code and open pull requests; **you approve the merge** —
the one hard gate. A single global **hub** — one per machine, serving every repo —
owns all state and mediates everything, so the CLI, the TUI, and every agent are
just thin clients of it.

This README is about *using* sindri. For the internal design, see `openspec/`.

---

## Install

Download the latest release and install it — a one-time step (after that,
`sindri upgrade` and the daily check keep you current). Both platforms install the
same binaries; pick your OS.

### Linux and macOS (tarball → `~/.local/bin`)

Download the tarball for your platform, extract it, and run the bundled `install.sh`
— it installs the binaries to `~/.local/bin` (and clears the Gatekeeper quarantine on
macOS, where the binaries are unsigned):

```bash
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m); [ "$arch" = x86_64 ] && arch=amd64; [ "$arch" = aarch64 ] && arch=arm64
url=$(curl -fsSL https://api.github.com/repos/flocko-motion/sindri/releases/latest | grep -o "https://[^\"]*_${os}_${arch}\.tar\.gz" | head -1)
curl -fsSL "$url" -o /tmp/sindri.tar.gz && tar -C /tmp -xzf /tmp/sindri.tar.gz && /tmp/sindri_*_${os}_${arch}/install.sh
```

Or grab it from the
[releases page](https://github.com/flocko-motion/sindri/releases/latest).

Ensure `~/.local/bin` is on your `PATH` (the installer says so if it isn't). podman
runs in a VM on macOS — `podman machine init` once, then `podman machine start`
(sindri auto-starts it after that).

**`~/.local/bin` is the only install location, deliberately.** There is no `.deb` or
other system package: one would land in `/usr/bin` and then shadow — or be shadowed
by — this install depending on PATH order, leaving two builds that drift apart
silently while the hub mounts tools from beside whichever it happens to be. Sindri
warns if it finds a second copy of itself on your PATH. If you installed an older
`.deb`, remove it once with `sudo apt remove sindri`.

That's it. The release bundles everything sindri ships — the `sindri` CLI/TUI, the
hub itself (`sindri-hub`, which `sindri hub start` runs), the agent browser
`sindri-worker` (it runs as `sindri` inside a pod), the `brokkr` toolbelt (code map +
linters), and `yq` — and needs the system tools **git** and **podman** present.

The one thing you bring yourself: **Claude credentials** at `~/.claude` (sindri
seeds them into the agent pods). The agent container image is built automatically
on first `sindri agent start` (needs network that once).

Agents also inherit what you've set up for yourself in `~/.claude`: your **skills**
(mounted live and read-only — host edits land without a relaunch) and your
**`keybindings.json`** (copied in at launch, so an edit applies to the next launch).
Both are optional; without them the agent runs Claude's defaults.

Then, in any repo, `sindri coauthor` gets you going in one command (see the Quick
start below) — it starts the hub for you, so you rarely launch one by hand.

**Optional extras** (sindri degrades gracefully without them, with a visible
note — never a hard failure): `openspec` (`npm i -g @fission-ai/openspec`) for the
spec-driven workflow, and the Go toolchain for the `deadcode` linter.

### Updating

Sindri checks for a newer release once a day (and on demand via **`sindri
upgrade`**); when there is one it points you at **`sindri-do-upgrade`** — a
one-shot script it drops in `~/.local/bin`. It fetches the latest tarball for your
platform and replaces the binaries in place, next to whichever `sindri` is running,
needing no elevated privileges. (The check can't replace the running binary itself,
so the install is a separate script.)

---

## Quick start

The simplest way in: **pair with an agent in your repo** — like running Claude
yourself, but sandboxed in a container. One command, in any git repo:

```bash
sindri coauthor
```

It starts everything it needs (the hub, a sandboxed pod, an agent
auto-named after a Norse dwarf) and drops you into its terminal. The coauthor
works on your **actual checkout** — you share the same files and drive it
directly, with no task queue and no PR gate. Detach with your tmux prefix then
`d` to leave it running; `sindri coauthor` again reattaches. That's the whole
thing.

### Advanced: managed workers and the task loop

When you'd rather hand off work than pair: put **one worker** on a task and merge
what it produces — you're the reviewer, no second agent needed.

```bash
sindri hub start --bg                       # start the global hub in the background
                                            # (foreground: `sindri hub start`; see also `sindri hub status` / `sindri hub stop`)

sindri task new "Add a /healthz endpoint"   # describe a task
sindri agent new                            # create a worker and start it — auto-named (e.g. dvalin)
                                            # it claims the task and starts coding
                                            # (--no-start to register the identity alone)

sindri tui                                  # watch the board live
```

Use the name `agent new` printed (or `sindri agent list` to see it). The first
start builds the agent image, so it takes a while. The worker writes code in its
own sandbox and opens a PR; review and land it:

```bash
sindri pr list
sindri pr approve pr-sd-abc123              # sign off (you're the reviewer)
sindri pr merge   pr-sd-abc123              # the one hard gate — human only
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
│            │  sindri hub  (one, all repos)     │                │
│            │  <state>/hub.db  (SQLite)         │                │
│            │  tasks · git · openspec · podman  │                │
│            └───────┬───────────────┬──────────┘                │
│         per-agent  │ unix socket   │ tmux send-keys             │
│            ┌───────▼───────┐   ┌───▼───────────┐                │
│            │ pod: dvalin   │   │ pod: reviewer │   …            │
│            │  Claude+tmux  │   │  Claude+tmux  │                │
│            └───────────────┘   └───────────────┘                │
└──────────────────────────────────────────────────────────────┘
```

- **Single writer.** The hub is the only thing that touches tasks, git, and
  `.sindri/`. Every UI reads `GET /state` and live-updates over `GET /events`.
- **Identity is the socket.** Each pod mounts one socket; the hub knows who's
  calling by which socket accepted the connection — no names on the wire.
- **The agent is a browser.** Inside a pod, the agent's command `sindri` has *no
  built-in commands*: run it with no arguments and the hub tells it the one thing
  to do next, filtered by role and state. A command it can't run is invisible.
- **You hold the gate.** Merge is human-only.

---

## Roles

Four roles — start more agents as you need them (`sindri agent new --role <role>`,
auto-named after Norse dwarves):

- **coauthor** — pairs with you in your own checkout, outside the task queue and the
  PR gate. What `sindri coauthor` starts, and the quickest way in.
- **worker** — builds: claims tasks, writes code, opens PRs.
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
# Build the feature: a parent with children. One agent takes the whole package.
sindri task new "Login feature" -t epic                          # → sd-LOGIN
sindri task new "Form UI"   --parent sd-LOGIN
sindri task new "Validation" --parent sd-LOGIN
```

A free agent picks up the marked container automatically: it goes on a standing
branch named for the container and starts on the first child. Then:

- The agent works a subtask, runs **`sindri checkpoint "…"`** → commits to
  the container branch, closes that child, and moves to the next — **no blocking**
  between subtasks.
- When you reach a milestone, **`sindri pr milestone <agent>`** captures the
  branch's current state as one PR and **blocks** the agent.
- You review it, then **`sindri pr approve pr-sd-LOGIN`** and
  **`sindri pr merge pr-sd-LOGIN`**. The merge lands, the branch is rebased onto
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
sindri pr info pr-sd-abc123          # metadata + diff
sindri pr lint pr-sd-abc123          # run the quality gate against the PR
sindri pr verify pr-sd-abc123        # check it out into a workspace to run by hand

sindri pr review pr-sd-abc123 "…"    # request an agentic review (assigns a reviewer)
sindri pr approve pr-sd-abc123       # approve it yourself (no reviewer needed)
sindri pr reject  pr-sd-abc123 "…"   # reject with feedback (routed to the worker)
sindri pr merge   pr-sd-abc123       # the hard gate — human only, requires approved
```

A worker's PR reaches `approved` via a reviewer agent **or** your own
`pr approve`. Merge always requires `approved`, and only a human merges.

Wondering why nothing is moving? **`sindri task next`** shows what would be
assigned next and why every other open task would not be; **`sindri pr next`**
answers the same for a reviewer's pool. Both take `--role` to ask on behalf of an
agent you have yet to start.

---

## When an agent needs you

Five status words mean an agent is stuck somewhere only a human reaches. The board
marks each one, and `sindri agent list` names them at the bottom:

| Status | What it means | What clears it |
|---|---|---|
| `blocked` | stopped at a prompt in its own session | answer it: `agent attach`, or `agent tell` |
| `escalated` | it asked you to decide something and stopped | `agent tell <name> "<answer>"` — it resumes itself |
| `signed-out` | its pane says to run `/login`, so anything typed there goes nowhere | log in on the host, then `agent restart <name>` |
| `full` | past its context window holding nothing | `agent clear-context <name>` |
| `stalled` | it holds work and its screen has stood still | `agent attach` to look, `agent tell` to prod |

**Escalation** is the agent's own verb: it stops on a decision that is yours to
make and says what it needs. Answering with `agent tell` clears the escalation
itself; `sindri agent resume <name>` releases one that cannot clear its own.

### Mail waits, tell interrupts

Two ways to reach an agent, separated by what they do to a running turn:

```bash
sindri agent mail <name> "when you get to it, note that X"   # waits to be read, never lost
sindri agent tell <name> "stop and look at Y"                # types into the live session now
```

Mail keeps until the agent next asks the hub what to do, which makes it the verb
for one that is down, restarting or signed out. Tell reaches a live session this
instant, and is lost if the agent is away. `sindri mail list` shows the fleet's
unread mail and `sindri mail show <id>` one message in full; reading marks it read
and nothing is ever deleted, so the mailbox stays the record of what an agent was
told.

### Retiring and clearing

- **`sindri agent retire <name>`** — assign it no further work; it finishes what it
  already holds. `--back` returns it to service.
- **`sindri agent clear-context <name>`** — arm a `/clear` that fires at the agent's
  next leaf boundary, so it never cuts into a task mid-flight. `--cancel` takes it
  back.

---

## Runs — one queue for the fleet

**`sindri run new <command…>`** queues a shell command into the single run slot the
agents share, so a suite you want run waits its turn instead of racing whatever an
agent is already running. It executes against a **copy** of the target, taken when
the run reaches the front, so nothing it writes touches the tree you are working in.

```bash
sindri run new go test ./...                    # this repo's checkout, uncommitted work included
sindri run new --agent dvalin go build ./...    # that agent's workspace instead
sindri run list                                 # then: info <id> · output <id> · cancel <id>
```

A run you queue goes ahead of every agent's, their submit gates included — you are
sitting there waiting on it and they are not. `sindri run priority <id>` re-orders
it afterwards.

---

## Tasks

Tasks live in the hub, which owns them. It also mirrors openspec changes and GitHub
issues, so a backlog can span all three; a repo carrying an existing `td` database
has it imported once, on first use.

```bash
sindri task new "Fix the parser" -t bug -p P1      # type: bug|feature|task|epic|chore
sindri task new "Sub-thing" --parent sd-abc123     # a child (subtask)
sindri task list
sindri task list --json                            # the same rows as JSON, for scripts (always an array)
sindri task info sd-abc123
sindri task edit sd-abc123 --parent sd-LOGIN       # move a task into a package
sindri task priority sd-abc123 P0                  # a priority is what releases it to a worker
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
  with a note if the `go` toolchain isn't on PATH. If the toolchain is *older* than
  `go.mod` requires — the go command then refuses to load anything — it says so and
  names the fix instead of reporting a broken build: in an agent pod that's
  **`go-upgrade`**, which installs the toolchain `go.mod` asks for (see below).
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
brokkr map internal/hub internal/ui     # several paths at once
brokkr map internal/ui --file tab_prs   # only files whose path matches
brokkr map --depth 1                    # bound how deep it descends
brokkr map --full                       # don't reduce, however long
```

Two searches, answering two different questions (both take a regex, both are
mutually exclusive):

```bash
brokkr map --find "func Merge"   # context: the DECLARATIONS enclosing a match
brokkr map --grep "func Merge"   # lines: the matching LINES, tagged with their decl
```

`--find` answers *what is this part of?* — it narrows the map to the types and
funcs that enclose a hit (plus the `var`/`const` that declares it, and any match
in the arch header, so a hit is never reported without a location).

`--grep` answers *where exactly is this?* — `path:line: text`, one match per line,
each tagged with the declaration it sits in:

```
codemap.go:27: var skipDirs = map[string]bool{…}  « var skipDirs
codemap.go:162: if skipDirs[d.Name()] {           « func write
```

That tag is the reason not to pipe a map through grep: you get grep's locations
*and* the structural context in one pass.

Both are smart-case (a lowercase pattern matches insensitively; any uppercase
letter makes it case-sensitive). If the full map runs past a line budget (default
1000, `--max`), it reduces to per-file headers only and tells you so — narrow the
scope or pass `--full`. Over-budget `--grep` truncates and reports the remainder
instead, since headers are no answer to a line search.

A third mode answers a third question — *where exactly is this declared?* — and
deliberately isn't a regex:

```bash
brokkr map --symbol Container   # just that func/type/var/const, wherever it's declared
```

`--symbol` is an exact Go identifier, always case-sensitive, never a substring
(`Foo` never matches `FooBar`) — a lookup, not a text search, mutually exclusive
with `--find`/`--grep`. Two receivers sharing a method name both come back; a
name declared inside a grouped `const (...)`/`var (...)` block is found even when
it isn't the first one. For where a symbol is *used* rather than declared, see
`brokkr refs`.

### Symbol references — `brokkr refs`

*Who calls this?* — the question that otherwise sends you back to `grep -rn`.

```bash
brokkr refs ProcessAlive              # whole tree
brokkr refs Tasks internal/hub        # scoped to a path
brokkr refs Merge --comments          # also where it's named in prose (ranked last)
brokkr refs Ref --limit 0             # no cap (default 200, tail-trimmed)
```

Each hit is classified by what the symbol is *doing* there, and the report is
**ranked, not file-ordered** — the definition first, then calls, then plain
references, with test files one step behind their own kind. So the top is the part
you asked about, and `--limit` trims the least relevant tail rather than the answer:

```
Tasks: 2 definitions · 1 call · 1 reference

── definitions ──
internal/hub/client/client.go:460:16: func (c *HTTP) Tasks() ([]store.Task, error) {  « hub/client / client · func (HTTP) Tasks
internal/ui/cli/hub.go:53:2: Tasks() ([]store.Task, error)  « ui/cli / commands · type backend

── calls ──
internal/ui/cli/task.go:180:21: tasks, err := b.Tasks()  « ui/cli / task · func taskListCmd

── references ──
internal/ui/cli/repo.go:198:28: d.Agents, d.OpenTasks, d.Tasks, …  « ui/cli / repo commands · func printRepoDetail
```

That classification is what a line search can't do: the interface declaration, the
method implementing it, a call, and a struct field read all match the same text.
Every hit carries where you landed — the file's arch-header `package:` and the
enclosing declaration.

The symbol is an **exact, case-sensitive identifier**, not a regex: `refs Foo`
never answers for `FooBar`. For patterns, use `map --grep` / `map --find` above.
Matching is syntactic (`go/ast`, no type checking), so two packages declaring the
same name both answer — scope it with a path or `--file`.

### An empty answer says which emptiness it is

`map`, its three searches and `refs` all read **Go only**, and they now say what they
actually scanned rather than going quiet — because "found nothing" and "read nothing"
are different answers, and only one of them is evidence:

```
$ brokkr refs Target ./a-typescript-repo
no Go files under ./a-typescript-repo — brokkr map and refs read Go only.

$ brokkr refs Nonexistent internal/hub
scanned 214 Go files, no match for Nonexistent.
note: refs matches an exact, case-sensitive identifier — …
```

The first is not absence: the tool never read those files. The second is, and it says
how much it read to earn the claim. `--find`, `--grep` and `--symbol` report the same
way (naming the pattern that missed) instead of exiting 0 in silence.

---

## Command reference

Orchestration is `sindri <category> <action>`; the toolbelt is the separate
`brokkr` binary.

| Category | Actions |
|---|---|
| `agent` | `list` · `new [name] [--role worker\|reviewer\|planner\|coauthor]` · `start` · `stop` · `restart` · `delete` · `mail <name> "msg"` · `tell <name> "msg"` · `attach` · `info` · `pane` · `dir` · `stats` · `memory <name> [size]` · `rebase` · `rebuild` · `plan <name> "goal"` · `clear-context <name> [--cancel]` · `retire <name> [--back]` · `resume <name>` |
| `task` | `list [--json]` · `new <title> [-t -p -d --labels --parent]` · `info <id>` · `edit <id>` · `priority <id> <P0..P4>` · `approve <id>` · `reject <id> "why"` · `unassign <id>` · `close <id>` · `delete <id>` · `reopen <id> "why"` · `comment <id> "text"` · `refresh` · `next [--agent\|--role]` |
| `pr` | `list` · `info <id>` · `lint <id>` · `verify <id>` · `review <id> "…"` · `approve <id>` · `reject <id> "…"` · `scrap <id>` · `milestone <agent>` · `merge <id>` · `next [--agent]` |
| `mail` | `list [--agent --filter unread\|all]` · `show <id>` |
| `run` | `new <command…> [--agent --priority]` · `list` · `info <id>` · `output <id>` · `priority <id>` · `cancel <id>` |
| `meeting` | `add <agent…>` · `remove <agent…>` · `join` · `log [-n]` · `new` · `close` |
| `repo` | `init` · `list` · `info [tag]` · `forget <tag>` · `config [k [v]]` · `color <tag> <n>` |
| `hub` | `start [--bg]` · `status` · `restart` · `stop` · `logs` |
| `brokkr` | `map [paths…] [--find --grep --symbol --file --depth]` · `refs <symbol> [paths…] [--comments --file --limit]` · `lint [deadcode\|loc\|comments\|openspec]` (none = all) |

Inside a pod the agent talks to the hub through a single command, **`sindri`**
(the browser binary, presented under that name in the isolated container) — run
with no args to get its next directive, or a verb the hub currently offers it:

| Role | Verbs |
|---|---|
| worker | `next` · `submit` · `contribute` · `revoke` · `checkpoint` · `resolve` · `rebase` · `git` · `run` |
| reviewer | `approve` · `reject` · `run` |
| planner | `create-task` · `edit-task` · `prioritise-task` · `reopen-task` · `openspec` · `state` · `approve` · `revoke` · `rebase` · `git` |
| coauthor | `git` · `run` |
| every role | `status` · `log` · `prs` · `show` · `lint` · `task` · `comment` · `escalate` · `resume` · `mail` · `meeting` |

`escalate` is how an agent stops on a decision that is yours to make; `mail` is
how it reads what it was sent. A verb it cannot run right now is invisible to it,
so the surface above is the maximum, filtered per agent by role and state.

---

## State & layout

| What | Where |
|---|---|
| Roster, workflow state, PRs, activity log | `<state>/hub.db` (SQLite) |
| Per-agent socket | `<state>/<project>/sockets/<name>/` |
| Agent Claude home | `<state>/<project>/agents/<name>/` |
| Code / commits | `.worktrees/<name>` (in the repo, host side) |
| Tasks | sindri's own store; `td` is imported once, then sindri owns them |
| Project config | `.sindri/config.yaml` (in the repo, yours to commit) |

`<state>` is `~/.local/state/sindri`, overridden by `$SINDRI_HOME` or
`$XDG_STATE_HOME`. Hub state is **central, not in your repo**: one hub serves every
repo, so nothing of its own lands in a checkout.

Throw a pod away freely; relaunch resumes from the activity log. Restart the hub
freely; nothing committed is lost.

```
cmd/sindri/         host CLI (agent/task/pr + tui); `sindri hub start` execs sindri-hub
cmd/sindri-hub/     the hub, as its own process (thin entrypoint over internal/hub)
cmd/sindri-worker/  the agent's thin browser (no command tree; `sindri` in a pod)
cmd/brokkr/         the toolbelt: code map + linters (no orchestration)
internal/api/       the exchange format: everything that crosses the wire
internal/hub/       the hub: workflow, store, command registry, agent lifecycle
internal/client/    thin hub client (CLI + TUI share it)
internal/adapter/   one package per external tool: git, container, tmux, tasks, herdr
internal/ui/        the front-ends: cli, tui, theme (shared rendering), attach
internal/brokkr/    the toolbelt's logic: lint + codemap
internal/container/ the agent image (embedded Dockerfile) + the runtime port
openspec/           the spec-driven design (specs + changes)
```

---

## Per-project config (`.sindri/config.yaml`)

A repo can declare project-specific settings in a committed `.sindri/config.yaml`, so
they travel with the repo. It overlays an optional global `config.yaml` in the sindri
state dir (`~/.local/state/sindri`, or `$SINDRI_HOME`), which overlays the built-in
defaults — **repo → global → default, per key**. All keys are optional:

```yaml
# .sindri/config.yaml
architecture: docs/ARCHITECTURE.md    # doc the reviewer must read (default: ARCHITECTURE.md)
containerfile: .sindri/Containerfile  # agent image recipe (highest-precedence; see below)
review_prompt: .sindri/review.md      # file whose contents become the reviewer's prompt
verify: scripts/verify.sh             # your own submit gate: an agent can't submit past it
github:
  issues: false                       # import open GitHub issues as tasks (default: true)
```

- **`architecture`** — repo-relative path to the doc the reviewer is told to read
  before every verdict, and which is injected into every agent's brief. When set it must
  exist. When unset sindri looks for `ARCHITECTURE.md`; if there's none it recommends —
  at hub startup, and in the Repos tab detail — that you point it at yours. It never
  creates the file.
- **`containerfile`** — repo-relative agent-image recipe (see the next section).
- **`review_prompt`** — repo-relative file whose contents replace the default reviewer
  prompt.
- **`verify`** — repo-relative executable the submit gate runs in the agent's worktree,
  after the rebase and **before the PR exists**, alongside the built-in checks. A
  non-zero exit refuses the submit and reports the output, so a PR that fails your build,
  your tests or your architecture tests is never created. It's a path rather than a
  command line so it can be validated before it runs — wrap a build tool in a script
  (this repo uses `scripts/verify.sh`, which runs `make verify`). A declared gate runs
  **whatever the language**; with no `verify` key the built-in Go checks apply exactly as
  they do today. Bounded by a timeout, with long output capped and the cut announced.
- **`github.issues`** — the repo's open GitHub issues are imported as `gh-<number>`
  tasks (via the `gh` CLI, reusing your `gh` auth). **On by default** (opt-out — set
  `false` to disable). Imported issues arrive **unrated**: they show in the backlog
  but a worker won't auto-claim one until you give it a priority (same as openspec
  items), so a repo's whole issue list never turns into surprise work. Degrades to no
  tasks when `gh` is missing / offline / the repo has no GitHub remote. On merge of a
  `gh-*` task's PR, sindri closes+comments the issue (best-effort — a merge is never
  blocked on GitHub).

**Fail-loud:** a malformed file, an unknown key, a wrong-typed value, or a path that's
absolute / escapes the repo / (when set) names a missing file makes the operation that
needs the project (launch, review, image build) fail with a clear error naming the file
and the problem — never a silent fallback. An **absent** config is fine; every default
holds.

---

## Customizing the agent image

Agents run in an image sindri builds from a recipe it carries embedded in the binary
(tagged `sindri-agent:latest`) — so a fresh install can build the image for any repo
without extra files. To add tools, pin a base, or bake in credentials, drop your own
`Containerfile` (or `Dockerfile`). It's discovered in this order — first match wins:

1. **Config key:** `containerfile:` in `.sindri/config.yaml` (above) — explicit.
2. **Per-repo:** `<repo>/.sindri/Containerfile` — applies to that repo's agents only.
3. **Global:** `<sindri state dir>/Containerfile` (`~/.local/state/sindri`, or
   `$SINDRI_HOME`) — applies to every repo you orchestrate.
4. Otherwise the **embedded default**.

Your recipe fully replaces the embedded Dockerfile, but it builds in the same
context: the entrypoint, docker shims, and helpers are materialized alongside it, so
`COPY sindri-agent.sh …` etc. still work. Easiest is to start from a copy of the
embedded recipe (`internal/container/buildctx/Dockerfile` in the source tree) and add
your layers. Keep the **agent contract** intact:

- a non-root `sindri` user (the recipe ends as `sindri` — write to `/home/sindri`,
  not `/etc`, unless you `USER root` then switch back);
- `/usr/local/bin/sindri` pointing at the mounted worker;
- the `sindri-agent` entrypoint and `WORKDIR /workspace`.

**Go toolchain in the pod.** The image's Go is whatever `golang:latest` had when it was
built, and that base pins `GOTOOLCHAIN=local` — so a repo whose `go.mod` moves ahead of
it makes every go command in the pod refuse to run ("go.mod requires go >= X"), which
looks like a broken build and isn't one. The image carries **`go-upgrade`** for exactly
that: it fetches the toolchain `go.mod` asks for (through go's own checksum-verified
module path, no root needed) and links it into `~/.local/bin`, which the recipe puts
first on `PATH`. Run it bare (`go-upgrade`), or with `latest` or an explicit `1.26.5`.
`brokkr lint deadcode` names it when it hits the refusal, so an agent gets the fix with
the failure; `sindri agent rebuild` is the other way out (a newer base image).

A custom recipe builds a content-derived tag `sindri-agent:custom-<hash>` (repos with
different recipes never clobber each other's tag or rebuild-thrash); the default stays
`sindri-agent:latest`. Editing the recipe (or a new ISO week) triggers a rebuild;
build errors are surfaced, not swallowed. Check which image an agent runs with
`sindri agent info <name>`.

Commit `.sindri/Containerfile` so the recipe travels with the repo (hub state lives
centrally under `~/.local/state/sindri`, not in the repo, so `.sindri/` is yours to
track).

---

## Building from source

For hacking on sindri (end users just install the tarball). Needs Go, plus `td`
and `yq` on `PATH` (they get bundled into the build).

```bash
make           # (or make help) list all targets
make install   # build sindri + sindri-hub + sindri-worker + brokkr, install to ~/.local/bin
make all       # + build the agent image too (needs podman)
make verify    # run the linters (the gate; release runs this first)
make check     # build + test + lint — the quality gate
make tarball   # build the release tarball into dist/
make release <major|minor|patch>   # lint, then release: push, open+merge a PR (gh), tag the merged default branch, return you to your branch (breaking|feature|fix aliases too)
```

---

## Acknowledgments

The Sindri TUI began as a fork of [sidecar](https://github.com/marcus/sidecar)
by Marcus; the current dashboard is a lean rewrite against the hub.
