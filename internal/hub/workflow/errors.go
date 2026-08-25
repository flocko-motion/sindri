// package: hub/workflow / errors
// type:    logic (the package's shared error vocabulary)
// job:     the sentinels a verb matches to tell an ordinary answer from a fault — an id nobody
// carries, an empty queue — so it can say so and let the caller carry on.
// limits:  the values and what they mean; each verb writes its own reply.
package workflow

import "errors"

// A missing id is a FACT about the backlog, and a verb that reports one as an error hands it to
// AgentExec as a hub failure — which stops the agent and escalates. balin was stranded that way for
// asking about a task outside the project its review put it in.
var (
	ErrNoSuchTask = errors.New("no such task")
	ErrNoSuchPR   = errors.New("no such PR")
	ErrNoOpenPRs  = errors.New("no open PRs")
)
