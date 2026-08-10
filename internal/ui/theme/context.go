// package: ui/theme / context
// type:    rendering (shared presentation primitives)
// job:     render an agent's live context size as one line, so the CLI and the TUI show the
// same figure in the same shape.
// limits:  formatting only; the number comes from the hub, already summed.
package theme

import "fmt"

// ContextLine renders an agent's context size ("241k tokens"), or a placeholder for an agent
// with no recorded usage yet (tokens == 0 — real usage is never exactly zero once a session has
// replied once).
func ContextLine(tokens int) string {
	if tokens == 0 {
		return "not yet measured"
	}
	return fmt.Sprintf("%dk tokens", tokens/1000)
}
