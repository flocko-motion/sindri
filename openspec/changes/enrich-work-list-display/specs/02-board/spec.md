## ADDED Requirements

### Requirement: Issues expose task hierarchy

Each `Issue` whose backing task has a `parent_id` SHALL expose that parent id,
and the board's refresh SHALL enrich tasks with the `parent_id` field td
reveals only on `td show --json` (since `td list --json` strips it). The board
SHALL also stamp each Issue with a `Depth` derived from its position in the
hierarchy (root = 0, child = 1, …) so renderers can indent uniformly.

#### Scenario: Refresh enriches parent_id

- **WHEN** the board refreshes
- **THEN** each Issue whose task has a parent has that `parent_id` populated,
  not left empty

#### Scenario: Children sit immediately after their parent

- **WHEN** the board is assembled with parent/child relationships
- **THEN** each child Issue appears immediately after its parent in the
  resulting `[]Issue`, in depth-first order, with `Depth` set to one more
  than its parent's
