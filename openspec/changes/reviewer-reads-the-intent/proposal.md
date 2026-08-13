# A reviewer cannot read what it is reviewing

## Why

A reviewer judged a diff against the architecture doc and its own taste. The one thing saying what
the work was FOR — the task — was unreadable to it:

- `DirReview` handed it the PR id and the task id AS A BARE STRING. `DirClaimed`, two functions
  below, gives the worker `"Claimed %s: %s"` — id and title.
- The `task` verb's roles were planner, coauthor and worker. A reviewer could not resolve that id.
- The agent-facing `show <pr-id>` printed the PR, the branch pair and the diff, and no task line —
  while the host's PR detail renders `task: <id> <title> (<status>)`.

So it could answer "is this good code that fits the architecture" and never "does this do what was
asked". Intent went unchecked by anyone but the human at merge.

There is a second casualty. `05-workflow` already requires that for a `spec:<name>` task "the
reviewer SHALL verify the diff against every requirement and scenario in the spec". The label lives
on the task. That requirement was not implementable with the surface the role had — and it turned
out the task view did not print labels at all, so granting access alone would not have fixed it.

## What changes

- The `task` verb accepts a reviewer, with whole-backlog scope. `CmdTasks` already scopes by role,
  so the hierarchy comes with it: `task <id>` prints parent and children, `task list` renders the
  tree.
- `task <id>` now prints the type and labels. That is what makes a `spec:` label discoverable, and
  it was missing for every role, not only this one.
- `DirReview` names the task by id AND title, and points at the verbs — the task, the backlog, and
  explicitly the comments, since that is where a plan is corrected while the body still describes
  the approach it replaced.
- `DefaultReviewPrompt` says the same. A directive covers one route into a review; the prompt holds
  however the review was requested.
- The agent-facing `show <pr-id>` names the linked task, matching the host view. The hub already
  resolves it, so this is rendering rather than new data.

## What it deliberately does not do

Reading grants no authority. A reviewer gains one read verb and nothing that proposes, edits,
re-orders or claims the work it judges — asserted against the registry's role lists, which is where
authority actually lives, rather than against the handlers.

It has been argued this needs a combined planner/reviewer role. It does not, and that would cost the
thing that makes the review worth having: a reviewer READING the plan keeps its independence, where
a planner grading its own spec would not.

## Impact

- Specs: `05-workflow` gains the requirement. It makes the existing spec-driven requirement
  implementable rather than restating it.
- Code: `internal/hub/commands.go` (the role list), `internal/hub/workflow/planner.go` (the scope,
  and type/labels in the task view), `prompts.go` (`DirReview`, `ReviewIntent`,
  `DefaultReviewPrompt`), `review.go` (the title lookup) and `pr.go` (`show`).
- `DirReview` gained a parameter. Its title lookup is best-effort: a title that will not load must
  not stop a review being handed out, and there is a test for the directive surviving that.
