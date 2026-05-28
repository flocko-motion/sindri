// package: main (sindri-review) / main
// type:    entrypoint
// job:     the reviewer agent's CLI used in the reviewer container (NOT GitHub);
//          dispatches the review command subset from internal/agentcli.
// limits:  command behavior lives in internal/agentcli; merge is human-only on
//          the host (-> cmd/sindri/pr.go).
package main

import "github.com/flo-at/sindri/internal/agentcli"

func main() {
	agentcli.Execute(agentcli.ReviewRoot())
}
