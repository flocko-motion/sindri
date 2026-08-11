# hub — delta

## ADDED Requirements

### Requirement: An agent is declared down only on repeated evidence

The hub SHALL decide agent liveness from an observer running on its own cadence, and a board read
SHALL report the latest observation rather than taking one.

No single observation SHALL declare an agent down, whatever its source. A failed liveness probe
and an absence from a pod listing SHALL each count as one reading, and an agent previously seen up
SHALL be reported up until consecutive failing readings reach the hub's threshold. A reading that
succeeds SHALL clear those accumulated failures.

Where the observer lists pods to decide whether they exist, it SHALL take that listing fresh
rather than from a cache shared with other callers. A listing describes the moment it was taken,
so a memoized one predating a launch reports the new pod absent.

An agent the observer has not yet reached SHALL NOT be reported down, since that is a claim no
observation supports. It SHALL be reported as not yet known, and such a non-observation SHALL NOT
retire a pending launch or stop intent as fulfilled.

#### Scenario: A pod created after the last listing

- **WHEN** an agent's pod is created between one observation and the next
- **THEN** the observer's own listing sees it, and the agent is not reported down

#### Scenario: One listing that missed the pod

- **WHEN** a single listing does not contain the pod of an agent last seen running
- **THEN** the agent is still reported up, and only repeated such readings report it down

#### Scenario: A stopped agent is eventually reported down

- **WHEN** an agent's pod is absent from consecutive listings reaching the threshold
- **THEN** the agent is reported down

#### Scenario: An agent nothing has observed

- **WHEN** an agent has been registered but no observation has been taken of it
- **THEN** its status reports that it is not yet known, rather than down

#### Scenario: An intent outlives a non-observation

- **WHEN** an agent under a pending stop intent has not been observed
- **THEN** the intent is not treated as fulfilled and still shows as pending
