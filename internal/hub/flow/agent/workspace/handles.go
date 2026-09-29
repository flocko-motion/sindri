// package: hub/flow/agent/workspace / handles
// type:    assembly (the verbs an agent types against its own workspace)
// job:     hold what `git` and `scratch` need — the hub's handles — so the two verbs that act on
// the agent's OWN tree sit together, apart from the actions the machine runs on its behalf
// (-> hub/flow/agent/act) and from the verbs that change a subject (-> hub/flow/pr, /task).
// limits:  carrying out a typed verb. Which verbs a state offers is the map's, what each is called
// is the catalogue's (-> hub/api/agents/verb), and nothing here decides where the agent
// stands next.
package workspace

import "github.com/flo-at/sindri/internal/hub/core"

// Act performs what an agent typed against its own workspace.
type Act struct{ *core.Core }

// New builds the workspace verb surface over the hub's handles.
func New(c *core.Core) *Act { return &Act{Core: c} }
