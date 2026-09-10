// package: hub/flow/agent / handles
// type:    assembly (the acting half of an agent's flow)
// job:     hold what acting on an agent needs — the hub's handles — so the four role maps beside
// this stay declarations and everything that WRITES is on one type.
// limits:  acting. What each role's states mean is its own package's, and this decides nothing.
package agent

import "github.com/flo-at/sindri/internal/hub/core"

// Act performs what an agent's flow decides. Separate from the maps so their purity is a property
// of the FILE rather than a discipline somebody has to remember (-> internal/arch).
type Act struct{ *core.Core }

// New builds the acting half over the hub's handles.
func New(c *core.Core) *Act { return &Act{Core: c} }
