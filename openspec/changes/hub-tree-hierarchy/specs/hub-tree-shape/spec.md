## ADDED Requirements

### Requirement: A package nests under the parent that owns it

A package SHALL sit under another only where that parent owns it or is its dominant consumer, and
where the parent's name is true for every caller. A package many peers read, with no single owner,
SHALL sit beside them under a grouping directory named for what its members ARE.

Subject matter alone SHALL NOT decide a parent: most of the hub concerns agents, so "it is
agent-related" distinguishes nothing.

#### Scenario: A shared leaf gets a grouping directory

- **WHEN** a package is read by packages in several different trees and owned by none of them
- **THEN** it sits under a directory named for the kind of thing it is, beside its peers, rather
  than under whichever tree happens to read it most

#### Scenario: A dominated package nests under its consumer

- **WHEN** nearly every importer of a package is inside one tree
- **THEN** it may nest under that tree, and the few outside importers reach it by its new path

#### Scenario: Nesting moves a directory, never merges files

- **WHEN** a package is nested under a new parent
- **THEN** it keeps its own package clause, its own tests and its own API, and only its import path
  changes

### Requirement: The hub's clock-driven work lives in one place

The hub's timer-driven work SHALL live under one directory: the sweep table, each sweep, and the
observer that polls the fleet, so what runs on a clock can be read off the tree. It SHALL reach the
rest of the hub through a declared seam rather than by sitting in the hub's own package.

#### Scenario: Every sweep is in one directory

- **WHEN** somebody asks what the hub does periodically
- **THEN** one directory answers, and no sweep runs from outside it

#### Scenario: The sweeps state what they need

- **WHEN** a sweep needs a service or a fact from the hub
- **THEN** it names it on the sweep package's own dependency interface, which the hub satisfies

### Requirement: A nested vocabulary package is not a state map

The flow tree's purity guard SHALL apply to the maps and conditions that decide, and SHALL NOT
apply to a vocabulary package nested under the tree for organisation. A directory judged by its
path alone would read such a package as a map and refuse its ordinary imports.

#### Scenario: A vocabulary package keeps its own imports

- **WHEN** a package of strings or shapes is nested under the flow tree
- **THEN** the guard skips it, and the maps beside it remain unable to reach anything that writes

#### Scenario: The maps are still held

- **WHEN** a state map is given an import that can write a row or steer a session
- **THEN** the guard fails, exactly as before the vocabulary package was nested
