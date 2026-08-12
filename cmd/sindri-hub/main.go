// package: main (sindri-hub) / main
// type:    entrypoint (thin)
// job:     run the hub as its own process: wire the container + coding-agent
// backends, open the hub, stamp its pid, refresh the pod binaries, and serve
// until signalled. `sindri hub start` execs or spawns this; nothing else does.
// limits:  no command tree, no flags — composition and Serve() only.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub"
	hubagent "github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/server"
)

// version is the build version, baked in via -ldflags "-X main.version=…" (matches
// the CLI's own stamp — see the Makefile). "dev" for a plain build.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sindri-hub:", err)
		os.Exit(1)
	}
}

func run() error {
	container.Use(chooseRuntime()) // wire the one container backend this process launches pods with
	agent.Use(claude.New())        // wire the one coding-agent backend

	h, err := hub.New()
	if err != nil {
		return err
	}
	defer h.Close()
	// Stamp this process (pid + build version) as the hub, so a second hub can't
	// start and clients can detect a stale-version hub.
	if err := server.WritePID(version); err != nil {
		return err
	}
	defer server.RemovePID()

	// How a rebuild reaches RUNNING agents. Async because copying tens of MB here
	// delayed the socket past the caller's readiness poll; Launch also syncs.
	go func() {
		if updated, serr := hubagent.SyncPodBin(); serr != nil {
			fmt.Fprintf(os.Stderr, "sindri-hub: pod-bin: %v\n", serr)
		} else if len(updated) > 0 {
			fmt.Fprintf(os.Stderr, "sindri-hub: pod-bin refreshed: %s\n", strings.Join(updated, ", "))
		}
	}()

	fmt.Fprintf(os.Stderr, "sindri hub %s listening at %s\n", version, h.SocketPath())
	// Recommend, don't impose: seeding a placeholder ARCHITECTURE.md littered repos
	// that never wanted one, so the hub says it once and leaves the choice.
	for _, line := range h.StartupAdvice() {
		fmt.Fprintf(os.Stderr, "sindri-hub: %s\n", line)
	}
	return h.Serve()
}
