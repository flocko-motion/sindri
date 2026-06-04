# Work List View — delta

## ADDED Requirements

### Requirement: Tab strip is the column header

The work list view SHALL render the view selector as the first row of
the content column — not as a separate selector elsewhere on screen.
The tab strip SHALL list every left-view tab (currently `[T]asks` and
`[W]orkers`), each with its hotkey letter shown in brackets so the
binding is discoverable from the tab itself. The active tab SHALL be
rendered bold + highlight; inactive tabs SHALL be dimmed. The view
SHALL NOT show any other view-name header (no separate green
"Tasks"/"Workers" line, no top-right view selector).

#### Scenario: Backlog view active

- **WHEN** the backlog (tasks) view is active
- **THEN** the column's first row reads `[T]asks  [W]orkers`
- **AND** `[T]asks` is rendered bold + highlight
- **AND** `[W]orkers` is dimmed

#### Scenario: Workers view active

- **WHEN** the workers view is active
- **THEN** the column's first row reads `[T]asks  [W]orkers`
- **AND** `[W]orkers` is rendered bold + highlight
- **AND** `[T]asks` is dimmed

#### Scenario: No duplicate view-name header

- **WHEN** the work list renders
- **THEN** the view name does NOT appear anywhere outside the tab strip
  — no separate green "Tasks"/"Workers" line above the rows, no
  view-selector duplicate on the title row
