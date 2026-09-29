// package: hub/messaging/mail / errors
// type:    logic (what a bad address is answered with)
// job:     say why a name did not resolve, in the words an agent can act on — which agent, which
// repo, and how to disambiguate when two of them share a name.
// limits:  the wording of a refusal; resolving is mail.go's.
package mail

import (
	"fmt"
	"strings"
)

// errNoSuchIn: a qualified address whose repo holds no such agent.
func errNoSuchIn(agent, repo string) error {
	return fmt.Errorf("no agent %q in %q — check the name and the repo", agent, repo)
}

// errNoSuchAnywhere: a bare name nobody in the fleet answers to.
func errNoSuchAnywhere(to string) error {
	return fmt.Errorf("no agent named %q anywhere in the fleet", to)
}

// errAmbiguous names every match, since the sender has to pick one and cannot see the roster.
func errAmbiguous(to string, qualified []string) error {
	return fmt.Errorf("%q is ambiguous — it names %s. Say which with `sindri mail %s <message>`",
		to, strings.Join(qualified, " and "), qualified[0])
}
