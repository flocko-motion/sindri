// package: hub/flow/pr / pool_act
// type:    logic (the reviewer pool spanning every project)
// job:     name the _global virtual project — the reviewer pool — and resolve which project a pooled
// reviewer currently works for. What it HOLDS is store.Store.ReviewingPR/RuledPRs, which
// every package reaches directly rather than through workflow.
// limits:  the name only; registering _global as a real project is elsewhere (-> sd-7374d3).
package pr

import ()
