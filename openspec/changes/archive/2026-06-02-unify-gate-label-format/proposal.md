# Gate label format is `render.GateLabel`, shared by CLI and TUI

## Why

The td bug report against the CLI ("require-review-code is shortened to
'code'") describes a pre-refactor state — the headless-render extraction
already brought CLI and TUI to "☑ review code" / "☐ review code" via
`render.Gates`. One divergence is left: `sindri pr review <id>` printed
each gate using raw `g.Name` ("review-code", dash kept). Same data, two
formats — the user's directive ("use the same codebase for formatting
labels in both CLI and TUI — always") makes that drift the bug to fix.

## What Changes

- New `render.GateLabel(g issue.Gate) string` is the single source of
  truth for one-gate display: `☑ review code` / `☐ review code`. Dash
  becomes space; the "review" content stays so the eye reads "review
  code", not just "code".
- `render.Gates` is now a thin loop over `GateLabel`, so list-style
  output stays identical to today (no golden churn) but cannot drift
  from single-gate output.
- `sindri pr review <id>` calls `render.GateLabel` instead of printing
  `g.Name` raw — gate text now matches every other surface.
- Unit test pins the format: `review-code` (un/approved) → "☐ review
  code" / "☑ review code"; `review-security` → "☐ review security".

## Impact

- Affected spec: `action-review-gate` — ADD a rendering requirement
  pinning the canonical format and naming `render.GateLabel` as the
  shared formatter.
- Affected code: `internal/render/render.go`, `cmd/sindri/pr.go`,
  `internal/render/render_test.go`.
- Goldens unchanged — the divergent surface (`pr review`) has no
  golden today, and the shared `Gates` output is byte-identical.
