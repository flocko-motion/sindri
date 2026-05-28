// package: main (sindri-worker) / main
// type:    entrypoint
// job:     the worker agent's CLI used in worker containers (NOT GitHub);
//          dispatches the worker command subset from internal/agentcli.
// limits:  command behavior lives in internal/agentcli; PR persistence in
//          internal/ghlocal/store; task I/O via the td CLI.
package main

import "github.com/flo-at/sindri/internal/agentcli"

func main() {
	agentcli.Execute(agentcli.WorkerRoot())
}
