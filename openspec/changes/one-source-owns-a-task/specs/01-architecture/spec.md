# Architecture — delta

## ADDED Requirements

### Requirement: A port resolves an owner rather than broadcasting

Where several adapters implement one port and each owns a disjoint set of subjects, the core SHALL
resolve the owning adapter and then ask it. It SHALL NOT call every adapter in turn and take
whichever one reports that the subject was its own.

A port method SHALL NOT return a "this was not mine" flag alongside its answer. An adapter that is
asked owns the subject, so its answer is the answer, and a subject no adapter owns SHALL be an error
raised once at the resolver rather than a loop that fell through.

#### Scenario: One adapter is asked

- **WHEN** the core acts on a subject a port covers
- **THEN** it resolves which adapter owns that subject and calls only that one

#### Scenario: An unowned subject fails once, clearly

- **WHEN** the core acts on a subject no adapter owns
- **THEN** it fails at the point of resolution naming the subject, rather than after asking every
  adapter and finding none admitted it

### Requirement: The core names no adapter

Code above the adapter layer SHALL NOT name a particular adapter, in an identifier or in a message
it produces, and SHALL NOT branch on a naming scheme that identifies which adapter is underneath.
Where the core needs a fact only one adapter can supply, the port SHALL carry it.

This SHALL be enforced by a test that fails the build, not by a comment on the port. Text the core
supplies to agents about a project's own conventions is not an adapter reference.

#### Scenario: A message does not name the adapter behind a subject

- **WHEN** the core refuses an operation because of where a subject's state lives
- **THEN** the refusal describes the situation without naming which adapter is underneath

#### Scenario: An undeclared adapter reference fails the build

- **WHEN** code above the adapter layer gains a reference to a particular adapter
- **THEN** the architecture test fails, naming the file and what it referenced
