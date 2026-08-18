# agent-runtime — delta

## ADDED Requirements

### Requirement: Language tooling is declared for every agent, and explains itself

Every agent's runtime SHALL be given the language tooling the image ships, unconditionally. What a
workspace contains SHALL NOT decide what tooling is declared.

The condition is the defect, not the cost. Tooling chosen from the tree is chosen once, when the
agent's home is prepared: a module below the workspace root reads as no project at all, and an agent
launched before its workspace is populated — a state the roster itself has a word for — loses the
tooling permanently, because the configuration persists across launches and nothing revisits it.
Whether a tree can be served is a question with a different answer per call, so it SHALL be asked
per call.

The tool the agent reaches for SHALL be the thing that answers. Where it cannot serve the tree, it
SHALL say what is wrong and what was looked for, and SHALL NOT return an empty result: a language
server answering "nothing found" reports a fact about the code, and a tree it never examined is not
that. Where the module is not at the workspace root, it SHALL look for one rather than assume its
absence.

A subcommand every agent depends on SHALL be discoverable in its tool's own help, whoever is
expected to invoke it.

#### Scenario: A workspace with no module at its root

- **WHEN** an agent's workspace holds its Go module in a subdirectory, or holds several modules
- **THEN** the Go tooling is declared and serves that module

#### Scenario: An agent launched before its workspace exists

- **WHEN** an agent's home is prepared before its workspace is populated
- **THEN** the tooling is declared anyway, and works once the workspace arrives

#### Scenario: A tree that genuinely has no module

- **WHEN** an agent asks a Go tool in a workspace with no module
- **THEN** it is told no module was found, where it was looked for, and that nothing was examined —
  rather than receiving an empty result

#### Scenario: The tool is findable

- **WHEN** an agent lists its toolbelt's commands
- **THEN** the command serving its language tooling is among them
