// package: hub/flow/run / handles_act
// type:    assembly (the acting half of a queued run's flow)
// job:     hold what running a command needs — the hub's handles — so the map beside this stays a
// declaration and everything that WRITES is on one type.
// limits:  acting. What a run's states mean is run.go's, and this decides nothing.
package run

import "github.com/flo-at/sindri/internal/hub/core"

// Act performs what a run's flow decides.
type Act struct{ *core.Core }

// New builds the acting half over the hub's handles.
func New(c *core.Core) *Act { return &Act{Core: c} }
