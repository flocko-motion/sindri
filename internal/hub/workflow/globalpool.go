// package: workflow / globalpool
// type:    logic (the reviewer pool spanning every project)
// job:     name the _global virtual project — the reviewer pool itself. Finding what a pooled
// reviewer holds, wherever that is filed, is store.Store.ReviewingPR/RuledPRs (every package
// that reads a reviewer's hold reaches those directly, not through workflow).
// limits:  the name only; registering _global as a real project is elsewhere (-> sd-7374d3).
package workflow

import "github.com/flo-at/sindri/internal/api"

// GlobalProject is the virtual project a fleet-wide reviewer lives in — api.GlobalProject under the
// name every call site in this package already uses; it crosses the wire, so it is defined there.
const GlobalProject = api.GlobalProject
