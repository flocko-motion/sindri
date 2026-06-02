# Action: Create Task — delta

## MODIFIED Requirements

### Requirement: Optional spec link

Creation SHALL support an optional spec link: attaching a `spec:<name>` label
at creation makes the task the implementing task for that spec. The link MAY
be supplied explicitly (e.g. `sindri task new --spec <name>`) or implicitly
by the invocation context (e.g. the TUI new-task modal opened from a
spec-only row inherits the row's spec).

#### Scenario: Linked create

- **WHEN** a task is created for spec `add-auth`
- **THEN** it carries the `spec:add-auth` label and pairs with that spec on the board

#### Scenario: Linked create from context

- **GIVEN** the invoker resolves a spec from its context (e.g. the cursor
  row in the TUI)
- **WHEN** a task is created without an explicit `--spec` flag
- **THEN** the resolved spec name is attached as a `spec:<name>` label, the
  same as an explicit link
