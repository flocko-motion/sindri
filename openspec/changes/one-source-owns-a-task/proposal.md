# One source owns a task, and says so — the port stops broadcasting

## Why

`tasks.go:14`, on the `Source` interface, states the rule:

> The hub never branches on which one is underneath.

It is declared there and enforced nowhere, and it is false in the core it describes.

**The mechanism is in the method signatures.** Three of the six return a "that wasn't mine" boolean:

    Finish(root, taskID string, scrap bool) (handled bool, err error)
    Comments(root, taskID string)           (cs []task.Comment, ok bool, err error)
    AddComment(root, taskID, body string)   (handled bool, err error)

and `OnMerged`'s own doc says *"callers notify every source blindly, so each acts only on its own
ids."* That is a broadcast loop wearing an interface's clothes. The core does not ask WHICH source
owns a task; it asks all of them in turn and takes whoever admits it. The polymorphism is only
apparent, because every caller still has to handle "not me".

A channel that can only carry yes/no cannot carry anything richer, so everything richer went around
it:

- **`Finish` degenerated to the lowest common denominator.** One boolean serves four events — a
  worker finishing a subtask, the user closing a task, the user scrapping one, and a merge. Three
  arrive as the same `false`, so the source guesses.
- **A source cannot say "I own this, and its work is not finished."** There is no such answer in the
  interface. So when a `checkpoint` closed `a-harness-command-answers` with **8 of its 10 tasks
  unticked**, the one component that knew stayed silent. The change was archived, the task vanished
  from the backlog on the next sync, the next checkpoint escalated its author over an id that no
  longer resolved, and it was recovered by hand. The hub had the number the whole time: it builds
  the listing title as `"%s (%d/%d)"` and had printed `a-harness-command-answers (0/10)`.
- **Where the interface could not express something, the core grew a branch.** `ReopenTask` and
  `TaskInfo` both branch on `task.IsOwned(id)`, and `ReopenTask`'s refusal names the adapters to the
  user: *"an openspec change or a GitHub issue"*.

**Why it went unnoticed.** `internal/arch` enforces exactly this kind of rule for two invariants —
who may inject into a session, who may poll the container runtime — by enumerating call sites and
failing the build on an undeclared one. There is no such guard for adapter knowledge in the core. A
rule that lives in a doc comment does not fail a build.

**What is NOT the problem, stated so it does not get "cleaned up".** `openspec submit` is a
planner-only verb and it belongs: a planner's landing verb, a worker's submit in different dress,
because a planner's output is spec changes. Its body is already source-agnostic — it uses
`PlannerBranch`, `HasChanges`, `Ahead`, `refuseIfBehind`, and gates through `qualityGate` rather than
calling a validator directly. Only three strings inside it name a tool. Most of the `openspec` text
elsewhere in the core is the agent brief telling agents where specs live, which is project
convention rather than the core knowing its adapters.

## What Changes

**Ownership becomes a question with one answer.** `Source` gains `Owns(taskID) bool`, and the core
resolves the owning source once instead of broadcasting. `handled`/`ok` disappear from `Finish`,
`Comments` and `AddComment`: a source that is asked owns the id, so its answer is the answer. An id
no source owns is an error at the resolver, in one place, rather than a loop that fell through.

**`Finish` carries the reason.** With one owner asked, the parameter can say which of the four
events happened — merged, subtask finished, closed by the user, scrapped — instead of collapsing
three into `false`. Each source decides per reason rather than guessing.

**A source can report that work is unfinished.** The close path asks the owner; the openspec source
answers from `CompletedTasks`/`TotalTasks`, the count the listing already shows. A source that
cannot measure says so and the close proceeds. This grants no source its own idea of when a task is
DONE — closing still ends it at its source, unchanged, for every source. It gives a source a way to
report a fact only it can see.

**The refusal is an ANSWER, not a fault.** An unfinished change is something a worker acts on:
finish what remains, or say why it no longer applies. Returned bare it reaches the agent as an
internal error and auto-escalates, which is what turned the second checkpoint into a stop.

**The two core branches go.** `ReopenTask` asks the owner whether it can reopen rather than testing
`IsOwned`; `TaskInfo` asks the owner where to read from. Neither names an adapter in a message.

**The verbs are renamed to what they do.** `checkpoint` becomes `done` — it ends the subtask and
takes the next. `contribute` keeps its job and gains a description that contrasts with it. Today a
worker looking for "save my progress" reads "checkpoint" and picks the terminal one, which is
precisely what happened; the author was mistaken about nothing except the vocabulary.

**An arch test holds the line.** The core names no task source: no `openspec`/`github`/`td` in a
message or an identifier under `internal/hub/workflow`, no branch on an id scheme. Declared on the
interface and checked by a build, the way injection and runtime polling already are.

## Impact

- Specs: `01-architecture` gains the rule that a port resolves an owner rather than broadcasting;
  `05-workflow` gains what may close a unit of work and what each verb commits to.
- Code: `internal/adapter/tasks/tasks.go` and the three sources, `internal/hub/workflow/close.go`,
  `reopen.go`, `task.go`, `feature.go`, `pr.go` (three strings), `internal/hub/commands.go`, the
  agent-facing text naming the renamed verbs, and `internal/arch`.
- `openspec submit` keeps its name. It is what planners type and what the brief teaches, and the
  fluency is worth more than the purity.

## Where this sits

Independent of `sd-badd07` (the harness seam) and workable beside it, with one caution: both touch
`internal/hub/workflow/task.go`. It is the same defect in a different port — a boolean at a seam
where a fact was needed — which is the third instance found this week.
