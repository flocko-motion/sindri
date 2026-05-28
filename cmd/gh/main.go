// package: main (gh) / main
// type:    entrypoint
// job:     sindri-local, the agent-facing workflow CLI used in containers (NOT
//          GitHub); dispatches the gh command tree (defined in this package).
// limits:  PR persistence lives in internal/ghlocal/store; task I/O via the td CLI.
package main

func main() {
	Execute()
}
