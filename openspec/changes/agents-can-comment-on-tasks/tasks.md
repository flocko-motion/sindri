# Tasks

## 1. Attribution

- [x] 1.1 `comments.Service.Add` gains an `author` parameter; the local-store
      path records it instead of the hardcoded `"user"`. The upstream-tracker
      path is unaffected — a source's own `AddComment` carries no author, since
      it posts as whichever identity the adapter authenticates with.
- [x] 1.2 The human path (`/task/comments/add`, `client.AddTaskComment`) threads
      `"user"` through explicitly via the request's existing `Source` field.

## 2. The verb

- [x] 2.1 `comment <id> <text...>`, registered for worker/reviewer/planner/
      coauthor, reusing `h.comments.Add`.
- [x] 2.2 Scope enforced in the handler, not trusted from the argument: a
      worker's id must be its held task or container; a reviewer's must be the
      task of the PR it is reviewing; a planner/coauthor's must exist in its
      project.
- [x] 2.3 State-filtered: hidden from a worker with no assignment and a
      reviewer with nothing to review (`Blocked`), matching the rest of the
      surface.
- [x] 2.4 Every refusal is agent-facing (`out` + exit code), not a hub-internal
      error — matching `AgentExec`'s convention that a returned `error` means a
      hub fault, not something the agent did.

## 3. Prompts

- [x] 3.1 The worker's system prompt names `sindri comment` as where a finding
      belongs, ahead of `sindri log`.

## 4. Spec

- [x] 4.1 Rename "Communication via comments" to "Communication via session
      injection" (content unchanged — it was always about inbound injection).
- [x] 4.2 Add "An agent can comment on a task", specifying the outbound
      capability and its per-role scoping.

## 5. Verify

- [x] 5.1 `make verify` passes.
- [x] 5.2 `openspec validate --all` passes.
