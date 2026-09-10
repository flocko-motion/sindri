// package: hub/flow/agent/verbs / handles
// type:    assembly (the verbs an agent types, over the hub's handles)
// job:     hold what an agent's own command surface needs, so `git`, `scratch` and `contribute`
// sit together as the things an agent RUNS — separate from the actions the machine runs on its
// behalf, which its map names (-> hub/flow/agent/act).
// limits:  carrying out a typed verb. Which verbs a state offers is the map's (-> flow/verb), and
// nothing here decides where the agent stands next.
package verbs

import (
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow/pr"
)

// Act performs what an agent typed.
type Act struct{ *core.Core }

// New builds the verb surface over the hub's handles.
func New(c *core.Core) *Act { return &Act{Core: c} }

// pr is the acting half of a pull request's flow: `contribute` exists to let one out.
func (a *Act) pr() *pr.Act { return pr.New(a.Core) }
