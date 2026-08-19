# Gate a commit, not a worktree — then cache the result

## Why

The gate costs minutes and paid that cost again for trees it had already checked.

It could not do otherwise. `repo.Gate` checked **whatever was on disk** in a worktree, and a result
about "whatever was on disk" cannot be stored: `PR.Lint`/`PR.LintAt` kept the output and a
TIMESTAMP, and a timestamp cannot say which tree was checked. So no stored result was ever trusted,
and the most ordinary sequence in the fleet — an agent lints, is told it passes, changes nothing,
submits — paid for the whole build-and-test twice.

Two smaller costs sat on the same path. `repo.Gate` ran the built-in linter and then the project's
verify script, which for this repo ends in `./bin/brokkr lint` — the same ~15s linter, twice per
gate. And the gate was only queued for `submit`/`contribute`: `sindri lint`, the reviewer's
`sindri lint <pr-id>`, and the periodic open-PR precheck each ran `repo.Gate` inline in the hub
process, so N agents linting at once meant N concurrent builds and test suites on the host,
competing with the queued gates the run queue exists to serialise.

## What changes

- **A gate describes a commit.** Every gate records the agent's workspace as a commit first, then
  checks that commit. Agents have no commit verb — the hub does all committing — so this is also
  where their work gets written down, at every gate rather than only after a passing submit.
- **A verdict is stored against the commit and reused.** A commit with a stored PASS is answered
  from the store: no queue, no build, no test run. The answer SAYS it was reused and names the
  commit, because a reused pass that reads like a fresh one is how an hour gets lost later.
  A stored FAILURE is kept for the PR view but never reused — a gate can fail for reasons outside
  the tree (a flaky test, a host under load), and pinning a commit to one bad run would need a
  human to undo it.
- **One linter per gate.** A declared `verify` owns the gate entirely; the built-in lint is what a
  project that declares none gets instead. The project's script is the thing that can run the
  built-in linter itself, and there is no reading of "the project asked for this command" under
  which sindri also knows better.
- **Every gate goes through the run queue.** `sindri lint` and the PR check join `submit` and
  `contribute` in the fleet's single slot, and so does the open-PR precheck — the third inline
  caller, which the task did not list. A precheck ranks as an ordinary run rather than ahead of
  them: it builds and tests like any gate, but nobody is held up by it.
- **`git restore` becomes `git rollback <id>`.** With the workspace recorded at every gate there are
  never unrecorded changes, so `restore` would be a permanent no-op that reads like a rescue. The
  replacement takes a point from `git history` and puts the workspace back there.
- **`check-go` leaves the gate.** `make verify` no longer curls `go.dev/VERSION`; CI and
  `make install` still do. It was a synchronous internet round trip on a path that runs dozens of
  times a day, checking something that moves every few weeks.

## Consequences worth stating

**A gate measures a checkout, not the tree the commit came from.** The obvious implementation gates
the agent's own worktree at the recorded commit — which is honest for `submit` and `contribute`, where
the agent is parked, and false for a self-check, where the hub tells the agent to carry on with other
work while the gate builds and tests for minutes. That gate would report violations about a
half-written file, and worse, file its verdict under a sha it was not taken on: an edit made during
the run and undone before the next gate leaves HEAD where it was, so a later submit reuses a pass that
never described that commit. So every gate with a commit checks a fresh detached checkout of it
(`.worktrees/gate`, the mechanism the PR check already used). The cost is one `worktree add` against a
warm build cache; the alternative was an invariant held only by an agent choosing to ignore its own
instructions.

**A PR check now checks the PR.** `LintPR` ran the gate in the author's live worktree, which after a
submit holds whatever the author has started since — so a reviewer could be shown a verdict about
code that is not in the PR. It now checks the commit the branch names, in a worktree of its own
(`.worktrees/gate`, not the review tree a human may be reading). That is also what makes the usual
case free: the submit gate passed that exact commit minutes earlier.

**`sindri lint` is no longer synchronous.** It returns at once with a queue position and the result
arrives as a message, like `sindri run`. The previous change (`queue-the-submit-gate`) explicitly
excluded it on the grounds that an interactive caller expects an immediate answer; that reasoning
does not survive the measurement — the answer takes minutes either way, and taking it inline meant
taking it N times at once. Caching softens it where it matters: an unchanged commit answers
instantly without queueing.

**A recorded failure decides nothing but reads back.** Never reusing a failure is right for a
DECISION — a flaky test would otherwise pin a commit until a human unpinned it — and wrong for a
READER: the message telling a reviewer its check has landed points at reading the verdict, and if that
re-ran the gate, the change whose purpose is to stop paying twice would hand its reviewers an
instruction that pays every time they look. So `GateVerdict` answers pass or fail for a reader, and
`GatePassed` (pass only) is what any landing stands on.

**A stored pass is keyed on the commit AND the verify command**, not on the config as a whole. A
project that re-points `verify:` invalidates its stored passes; one that edits the script the key
points at does not. The commit is the tree, the key is the question asked of it, and the script's
own contents are outside both — a limit worth knowing rather than hiding.

**Two askers, one check, both told.** A second request for a check already queued joins it — but the
run carries one asker, so the joining agent had to be recorded somewhere: `run_waiters` holds every
agent that asked, and the completion messages the list. Re-pointing the run at the newcomer instead
would have cost the human who queued it their place at the front of the queue, and would still have
left an agent-joins-agent silent.

**Neither destructive git verb may touch a shared checkout.** A coauthor's `/workspace` IS the user's
own tree, so a rollback there would be `reset --hard` plus `clean -fd` over whatever they have in
progress. `git drop` had the same hazard already (it commits into that tree), and both are now
refused the way `RebaseAgent` has always refused it. The gate itself follows the same rule: it
records a commit only in a worktree the hub owns — never the user's checkout, and never a reviewer's
detached look at somebody else's branch, where a clean tree is named by its HEAD and a touched one is
checked as it stands and recorded against nothing.

**Rollback rewinds history rather than recording an undo.** `git drop` reverts paths by committing
the reversion; a rollback moves the branch back. Nothing is published (a merge squashes the branch),
the discarded commits are the gate's own checkpoints, and the alternative — a commit whose tree
matches an earlier one — cannot express "and stop keeping what came after". The verb refuses
anything that is not the agent's own: an id, on this branch, no earlier than where the branch left
the reference.

## Not done here

- The gate still runs on the HOST, not in a container. Moving it is a separate question, and this
  change is about what it checks and how often, not where.
- `revoke` still cannot withdraw a queued gate — out of scope when the gate was queued, out of scope
  now.

## Impact

- Specs: `03-gh-local` (the submit gate), `hub` (the agent's git surface), `project-config` (which
  gate owns the linter).
- Code: `internal/hub/workflow/{gate,pr,contribute,prcheck,run,execrun,gitcmd,prompts}.go`,
  `internal/hub/repo/repo.go`, `internal/hub/store/{workflow,runs,store}.go`,
  `internal/adapter/git/{git,restore}.go`, `internal/hub/commands.go`, `internal/api/{pr,run}.go`,
  `internal/ui/tui/tab_prs.go`, the pod git shim, `Makefile`, `.github/workflows/ci.yml`.
- Store: two new columns (`pr_lint.sha`, `runs.commit_sha`) and two new tables (`gate_result`,
  `run_waiters`), all additive.
