# The layer rule names the port, not the tool

## Why

The two binding documents contradict each other, and the code can only satisfy one.
`ARCHITECTURE.md` says "The core calls adapters"; `01-architecture`'s layer table says
a `logic` file "MUST NOT import ... adapters" and reserves composing adapters and
logic for `assembly`. So 18 files typed `logic` import an adapter and are in
violation of a rule they cannot follow — `hub/workflow/{engine,task,pr,merge,resolve,
scrap,reference,contribute,gitcmd,importtd}.go`, `hub/repo/{repo,merge,rebase}.go`,
`hub/agent/{lifecycle,inject,runtime}.go`, `hub/comments/service.go`,
`hub/credwatch.go` — while no file in the repository is typed `assembly` or
`rendering` at all.

The rule is badly worded rather than wrong. What it means to forbid is the core
naming a *concrete tool*: the core should depend on the abstraction and let a
composition root choose the implementation. Read that way, the codebase is mostly
right and precisely wrong in a few places. Three ports already exist and are used as
intended — `internal/container` (two backends behind it), `internal/adapter/agent`,
and `internal/adapter/tasks`. The failures are where the core reaches around a port
that exists:

- `hub/workflow/engine.go:23-24` — `taskSources` returns `[]tasks.Source{...,
  spec.Source{}, github.Source{}}`, naming two concrete sources one line below the
  port it returns.
- `hub/comments/service.go:84,87,150,156` — `github.Number`, `github.AddComment`,
  `github.IssueComments`, so the unified comment sync is written against GitHub and a
  second tracker means editing the core.
- `hub/workflow/pr.go:278` — `spec.Validate(wt)` in the merge path.

Separately, the requirement that every external tool is reached through an adapter is
simply unmet in four places, all of them the core shelling out directly:
`hub/server/pidfile.go:83,96` (`ps`, `lsof`), `hub/repo/repo.go:52` (the `brokkr`
binary), and `update/update.go:209,221` (`gh` — duplicating
`internal/adapter/tasks/github`, which already wraps it). And `hub/agent/lifecycle.go:147`
prints a warning to stdout from the core, which is headless by rule.

Two conformance rules also need to match reality rather than the reverse. Ten files
declare a `type:` outside the seven legal values — `persistence` four times, plus
`small shared helpers`, `headless helper`, `dev/test harness`, `dev tool`,
`composition root` and `application helper`; `internal/hub/store` is itself split,
four files `persistence` and two `logic`. Nothing enforces the enumeration, because
`brokkr lint comments` checks that the four header fields are present and bounded,
not what they say. And 112 of 149 files have a `limits:` field with no `-> package X`
pointer, which the spec states as the convention — a convention three quarters of the
codebase does not follow is a convention that needs rewording, not a 112-file diff
that changes no behaviour.

## What Changes

- **The rule names the port.** The core SHALL depend on the abstraction wherever a
  family of implementations exists, and SHALL NOT name a concrete adapter; choosing
  the implementation is the composition root's job. A `logic` file importing a *port*
  is correct and always was.
- **Single-implementation tools are imported directly, by rule.** git and tmux have
  one implementation each and no plausible second; the core imports
  `internal/adapter/git` and `internal/adapter/tmux` concretely and the spec says so,
  rather than requiring ceremony that buys nothing.
- **The three reach-arounds are closed.** The task-source set is injected from the
  composition root; comment sync goes through the task-source port; spec validation
  goes behind a quality-gate abstraction. (`importtd.go` keeps naming td: it *is* a
  one-way td migration, and naming the thing being migrated from is honest.)
- **The tools the core shells out to get adapters**: `ps`/`lsof` behind a process
  adapter, the `brokkr` binary behind a lint-gate adapter, and `gh` in `update`
  routed through the existing GitHub adapter instead of a second call site.
- **The core prints nothing.** The one stdout warning becomes a logged event.
- **The seven layer types become a closed, checked set**, with `persistence` folded
  into `adapter` (a SQLite store is an adapter over an external store) and the six
  one-off labels replaced. A project test asserts the vocabulary, the same way the
  import-guard test asserts the front-end boundary — sindri's own rules stay in
  sindri's own tests rather than being added to a generic toolbelt. The test is the
  cheap half of enforcement: it catches a mechanical breach before review, while the
  reviewer agent catches what needs judgement.
- **`ARCHITECTURE.md` is corrected first, because it is the gate.** The reviewer is
  told to read the architecture doc before every verdict and it is injected into every
  agent's brief (see `project-config`), so a stale doc means every review is measured
  against rules that no longer hold. Its wording of this rule — "The core calls
  adapters" — is what this change fixes, and it is fixed before the code it governs is
  touched, so the work that follows is reviewed against the rule it is meant to
  satisfy. The doc's *layout* lines are corrected separately, alongside the change that
  alters the layout, so the doc never describes packages that do not exist yet.
- **The `limits:` convention is reworded** to what it is actually for: naming the
  neighbour that owns an excluded concern *when there is one*, rather than requiring a
  pointer in every header.

## Capabilities

### Modified Capabilities

- `01-architecture`: external adapters are isolated — the core depends on the port
  where a family exists, names no concrete adapter, and single-implementation tools
  are a stated exception; every external tool is reached through an adapter, with no
  direct shelling out from the core. Layer types are a closed, checked set. Headless
  logic writes no terminal output. File headers point at a neighbour when one owns the
  excluded concern.

## Impact

- **Source of truth:** `internal/hub/workflow/engine.go` (injected sources),
  `internal/hub/comments/service.go` (through the port),
  `internal/hub/workflow/pr.go` (gate abstraction), new adapters for the process
  probe and the lint gate, `internal/update/update.go` (reuse the GitHub adapter),
  `internal/hub/agent/lifecycle.go:147` (log, not print), the ten header `type:`
  fields, and a new vocabulary test.
- **`ARCHITECTURE.md`** carries the summary of this rule and is corrected alongside it
  — the sentence "The core calls adapters" gains "through their ports", and the
  document's stale package names are fixed in the same pass.
- No behaviour change for a user: this is the rule and the layering it describes.
