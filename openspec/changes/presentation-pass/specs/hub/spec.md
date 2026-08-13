# hub — delta

## ADDED Requirements

### Requirement: A PR's lifecycle is a classified projection of its log

Every event logged against a PR SHALL be classified, in the exchange package, as either
a lifecycle milestone — what happened to the PR — or a diagnostic, which explains how
something went. The classification SHALL be exhaustive: an event type nobody has
classified SHALL fail the build rather than fall to a default, since either default is
wrong in silence — hidden loses a state from every view, shown clutters the summary
that exists to be short.

From that classification the exchange package SHALL derive a PR's lifecycle: the
milestones in order, each with who acted and when. A verdict's author and time SHALL
come from the review record, which holds them as data, rather than from the event
payload, which says the same thing as prose.

Every front-end SHALL show this summary in a PR's detail, above the diff, and SHALL
keep the full event log as well: the summary answers "where has this got to", and the
diagnostics it drops are exactly what a failure needs.

#### Scenario: A PR's detail opens with its story

- **WHEN** a user opens a PR's detail in either front-end
- **THEN** it shows the lifecycle — created, the verdicts with their authors, merged —
  before the diff, with the full event log still available below

#### Scenario: A diagnostic stays in the log

- **WHEN** a PR has precheck, conflict or review-plumbing events
- **THEN** they appear in the full log and not in the lifecycle summary

#### Scenario: An unclassified event type

- **WHEN** a new event type is logged against a PR without being classified
- **THEN** the build fails, naming the type and asking which of the two it is
