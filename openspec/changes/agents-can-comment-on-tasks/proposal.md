# Agents can comment on their tasks

## Why

An agent cannot comment on a task. The capability exists and is fully wired
hub-side — `AddTaskComment`, backed by the unified comment service in
`internal/hub/comments` — but the only callers are the two front-ends
(`sindri task comment`, the TUI's comment input). No agent role has a comment
verb. This is an omission, not a decision: nothing in the specs forbids it, and
the machinery is already there.

The hub-routing capability makes acting on a shared object the one sanctioned
way an agent communicates durably. A task is that object, and it is the one an
agent could not touch. So a worker that discovers the task body is wrong, hits a
blocker, or makes a decision worth recording had nowhere to put it that the next
reader of the task would see — it landed in the PR (about the diff) or in
`sindri log` (the agent's own audit trail, explicitly not discussion, and
invisible to anyone reading the task). For a `gh-` task the loss was external
too: the human path comments upstream on the GitHub issue; an agent's finding
never reached it. Feedback was one-directional — reviewer rejections and `tell`
reach an agent; nothing an agent learns reached the task.

Separately, every local comment was attributed to the literal string `"user"`
regardless of who or what actually wrote it — `comments.Add` took no caller
identity at all. That bug would have made a real agent comment misleading.

The `05-workflow` requirement titled "Communication via comments" is actually
about the hub injecting messages into a session (`tell`, reject-feedback) — the
title and the content disagree, and neither covers an agent writing a comment.

## What Changes

- A `comment` verb, reachable by workers, reviewers, planners and coauthors,
  reusing the existing comment service — no new comment path, and `gh-`
  write-back comes free (the source's own `AddComment` already posts upstream).
- Scoped by what the role can already see: a worker its own task or the
  container it holds; a reviewer the task of the PR it is reviewing; a planner
  or coauthor any task in its project, since they already read the backlog. The
  verb is state-filtered (absent from a worker's surface with no assignment,
  from a reviewer's with nothing to review) and enforces the same scope inside
  its handler, independent of what the caller claims.
- The id is optional wherever that scope resolves to one task: a worker writes
  `comment <text...>` and it lands on the task it is working, a reviewer's on the
  task under review. A planner and a coauthor still name one. Since the surface
  is state-filtered, the help is too — a role is shown only the arguments it
  supplies — so `registry.Command` gains a per-caller `HelpFor`. Inside a feature
  the bare form means the subtask, the container is reached by its id, and the
  reply names which of the two it wrote to.
- `comments.Add` gains an `author` parameter, threaded from the caller's own
  identity rather than a hardcoded `"user"` — fixing the local-comment
  attribution bug for the human path too (it now sends `"user"` explicitly
  where it used to send nothing).
- The requirement titled "Communication via comments" is renamed to
  "Communication via session injection" (its content is unchanged — it was
  always about inbound injection); a new requirement, "An agent can comment on
  a task", specifies the outbound capability and its per-role scoping.
- The worker's system prompt names `sindri comment` as where a finding belongs,
  ahead of the activity log.

## Impact

- **Source of truth:** `internal/hub/comments/service.go` (`Add`'s new
  `author` param), `internal/hub/commands.go` (`comment` verb + scope check +
  per-role argument form), `internal/hub/registry/registry.go` (`HelpFor`),
  `internal/hub/task/ids.go` (`IsID`, the whole-string id check the optional
  argument needs),
  `internal/hub/server.go` and `internal/client/client.go` (the human path now
  threads `"user"` explicitly through the same field).
- No wire format change beyond `TellReq.Source` now being read on this route
  (it already existed, for `tell`, and was simply ignored here before).
