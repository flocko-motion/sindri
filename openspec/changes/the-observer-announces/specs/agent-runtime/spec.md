# Agent runtime — delta

## ADDED Requirements

### Requirement: The observer announces a settled change

The observer SHALL announce an agent's observation when a settled reading differs from the last one
it announced, carrying both the previous and the new reading so a subscriber need not re-derive what
changed. It SHALL announce from the one place every observation is recorded, so there is a single
announcer and no second source of truth.

It SHALL announce the SETTLED reading — the one the hub is prepared to stand behind, after the
consecutive-failure and stillness dampening it already applies — and never the raw per-beat sample.

Announcing SHALL NOT be able to block or fail the observation sweep, on which the whole hub's
liveness depends. A slow or failing subscriber SHALL cost the announcement, never the beat.

#### Scenario: A change is announced once

- **WHEN** an agent's settled observation differs from the last announced reading
- **THEN** the change is announced once, carrying the previous reading and the new one

#### Scenario: An unchanged reading announces nothing

- **WHEN** the observer takes a reading that matches the last announced one
- **THEN** nothing is announced, however many beats pass

#### Scenario: A flapping agent does not announce per beat

- **WHEN** an agent's raw sample changes on consecutive beats but the settled reading does not
- **THEN** no announcement is made until the settled reading itself changes

#### Scenario: A subscriber cannot stall the observer

- **WHEN** a subscriber is slow or returns an error while being announced to
- **THEN** the observation sweep continues on its beat, and the failure is recorded rather than
  propagated
