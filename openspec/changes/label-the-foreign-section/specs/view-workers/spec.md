# view-workers — delta

## ADDED Requirements

### Requirement: A fleet-wide listing labels what waits on you elsewhere

`sindri agent list` and `sindri pr list` SHALL label what waits on the user outside the repo the
command was run in. Both listings are fleet-wide: they show every repo, and a row is placed only by
its repo column. When any listed row waits on the user elsewhere, the listing SHALL open with those
rows under a heading naming and counting them, then the rows of the repo it was run in under their
own heading, then the rest of the fleet under a third — each separated by a blank line. This is the
reading the TUI's scoped lists teach, so a user moving between the two front-ends learns it once.

The remaining rows SHALL NOT be labelled as the local repo's. The listing is fleet-wide, so what is
left once the waiting rows are lifted out spans every repo, and a heading claiming otherwise would be
the misreading this exists to prevent.

No heading SHALL appear when nothing waits on the user outside the repo the command was run in, and
the rows SHALL then print in the order the shared sort gave them. A listing is read far more often
than that case arises, and it must not be rearranged to explain a group it does not hold. Outside any
registered repo there is no local repo for a row to be foreign to, and the listing SHALL likewise
stay flat.

A heading with no rows under it SHALL NOT be printed, since it reads as a group whose rows failed to
appear.

#### Scenario: An agent stuck in another repo

- **WHEN** `sindri agent list` runs in one repo and an agent in another repo is waiting on the user
- **THEN** that agent is listed first, under a heading stating how many wait elsewhere, above the
  headed rows of the current repo and of the rest of the fleet

#### Scenario: Nothing waits elsewhere

- **WHEN** every agent or PR waiting on the user is in the repo the command was run in
- **THEN** the listing prints its rows flat, in the sort's order, with no headings

#### Scenario: Run outside a repo

- **WHEN** `sindri pr list` runs where no registered repo contains the working directory
- **THEN** the listing prints flat, since no row can be foreign with nothing to be foreign to
