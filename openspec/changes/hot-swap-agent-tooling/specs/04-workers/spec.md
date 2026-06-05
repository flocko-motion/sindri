# Workers — delta

## ADDED Requirements

### Requirement: Agent instructions are carried by the binary

An agent's instructions SHALL be emitted by its role binary via an `init`
command, role-bound at command-tree registration. `init` SHALL take no
arguments: its role is fixed by which binary it is built into, and `mode` is read
from the launch environment (set from the agent's index entry). The launcher SHALL inject only a single, stable bootstrap
instruction telling the agent to run `init` and follow its output. Agent
instructions SHALL NOT be baked into the container image as skills or `CLAUDE.md`.

#### Scenario: Worker bootstrap

- **WHEN** a worker container starts
- **THEN** the agent is told to run `sindri-worker init` and the binary emits the
  worker instructions, reading its mode from the environment if needed

#### Scenario: Reviewer bootstrap

- **WHEN** the reviewer container starts
- **THEN** the agent is told to run `sindri-review init` and the binary emits the
  reviewer instructions

#### Scenario: No baked instructions

- **WHEN** the image is built
- **THEN** it contains no skills directory and no agent `CLAUDE.md`; instructions
  come only from the mounted binary

### Requirement: Hot-swappable agent binary

Each role binary SHALL be installed into its own host directory and that
*directory* SHALL be bind-mounted into the container (not the binary file), so an
atomic replace on the host (`mv`) is visible to the next `exec` in a running
container. Updating an agent's binary or instructions SHALL NOT require an image
rebuild or a container restart.

#### Scenario: Swap while running

- **WHEN** a role binary is rebuilt and `mv`-installed into its directory while a
  container is running
- **THEN** the next invocation of that binary inside the container runs the new
  version, with no restart and no image rebuild

## MODIFIED Requirements

### Requirement: Bundled agent tooling

The container image SHALL bundle the role-agnostic tools an agent needs — `td`
and the openspec CLI — so an agent can drive its loop offline. Each role's sindri
binary SHALL be provided by bind-mounting its host directory and placing that
directory on `PATH`, so the binary can be updated without rebuilding the image: a
worker container gets `sindri-worker`, the reviewer container gets `sindri-review`.

#### Scenario: Worker runs the loop

- **WHEN** a worker container starts
- **THEN** `sindri-worker`, `td`, and `openspec` are all on its PATH

#### Scenario: Reviewer runs the loop

- **WHEN** the reviewer container starts
- **THEN** `sindri-review`, `td`, and `openspec` are all on its PATH
