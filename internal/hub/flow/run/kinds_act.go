// package: hub/flow/run / kinds_act
// type:    logic (what a run is FOR)
// job:     the kinds a run can carry and the two questions the queue asks about one — whether it
// belongs to a pull request, and whether anybody is blocked waiting on it.
// limits:  naming and ranking. What a kind's verdict then UNLOCKS is the subject's (-> flow/pr).
package run

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/prompts"
)

// gateOnAPR reports a gate whose subject is a PR, not an agent's workspace: nothing to go stale,
// and nobody to report back to.
func gateOnAPR(r api.Run) bool { return r.Kind == GateLintPR || r.Kind == GatePrecheck }

// gateBlocksSomeone reports a gate somebody waits on. The advisory precheck is nobody's blocker, so
// it queues as an ordinary run would.
func gateBlocksSomeone(r api.Run) bool { return r.Kind != "" && r.Kind != GatePrecheck }

// GateSubject names what a gate checked. A shared checkout has no commit to name (-> GateCommit),
// and calling it one would be a lie a reader could act on.
func GateSubject(sha string) string {
	if sha == "" {
		return "the working tree as it stands"
	}
	return prompts.ShortSHA(sha)
}

// The gate kinds. A run's Kind is what its result unlocks.
const (
	GateSubmit     = "submit"
	GateContribute = "contribute"
	GateLint       = "lint"
	GateLintPR     = "lint-pr"
	GatePrecheck   = "precheck"
)
