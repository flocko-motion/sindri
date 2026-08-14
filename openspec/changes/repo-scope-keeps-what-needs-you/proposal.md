# Repo scope keeps background work that needs you, from any repo

## Why

Repo scope means "this repo, plus background work anywhere that needs the user". Agents and PRs run
while attention is elsewhere, so when one ends up waiting on the user it has to surface wherever
they are. Today only one of the two does.

The Agents tab already makes the exception — `agentVisible` admits "the active repo's agents, plus
any agent stuck on the user", and the repo key gathers a stuck foreign agent into its own group. The
PRs tab has no exception at all: `if !m.inScope(p.Project)`. So an approved PR in another repo — the
one action only the user can take — is invisible until they switch repos, and nothing tells them to
switch. Worse, the attention marker beside the handle is fleet-wide, so the user is told something
needs them and shown a list where nothing does.

## What changes

- `prVisible` mirrors `agentVisible`: the active repo's PRs, plus any PR waiting on the user
  anywhere. It CALLS `api.PRNeedsUser` rather than restating the condition — three places now decide
  that question (the marker, the row colour, and now visibility), and separate derivations drift
  invisibly, since each looks plausible alone.
- The tab badge counts what the list shows, through the same predicate, so the header cannot
  contradict the rows beneath it.
- `api.SortedPRs` groups PRs by repo, the same key `SortedAgents` uses and the same order the Repos
  list shows. Interleaved by age, a foreign row reads as a filter that has stopped working; grouped
  under its own repo it reads as what it is. One sort, called by both front-ends.
- The scope label is no longer `repo`, which claimed to exclude exactly what the scope shows. It
  reads `repo+needs-you` — still repo-first, and honest about the exception. This corrects the
  Agents tab's label too, which has had the exception all along under a word that denied it.
- `sindri pr list` calls the same sort, and gains the repo column `agent list` already has: that
  listing is fleet-wide, so without it the rows it gathers from elsewhere are unplaceable.
- `api.RepoName` is the one tag→name lookup both front-ends use. The TUI had it privately and the
  CLI needed it; a row placed by one name in one front-end and another name in the other is a row
  the user cannot follow between them.

## Non-goals

The Tasks tab is deliberately unchanged, and the hub capability that it "SHALL be scoped to the
currently selected repo" stands. A proposal awaiting a verdict is FOREGROUND work — created in the
repo the user is already looking at, and seen because they are there. Only background work needs an
alert that crosses repos.

## Impact

- Specs: `view-tui`'s scope-toggle requirement said the narrow scope shows "only the active repo's
  entries". That was already untrue of Agents and is now untrue of PRs, so the delta MODIFIES it
  rather than leaving the spec describing a rule the code does not follow.
- Code: `internal/ui/tui/items.go` (`prVisible`, the badge, the label), `tab_prs.go`, `util.go`,
  `internal/ui/cli/hub.go`, `internal/api/prorder.go` (new), `internal/api/project.go`.
- `TestTabCountFollowsScope`'s fixture gains one of each foreign kind — a PR with a live reviewer in
  its own repo, and an approved one — because "the badge follows scope" now needs both to mean
  anything.
