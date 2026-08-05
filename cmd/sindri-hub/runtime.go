// package: main (sindri-hub) / runtime
// type:    composition root (wires the container backend)
// job:     pick the ONE container runtime this process uses and inject it into the
// container port — the hub's own copy of cmd/sindri's choice, since the hub is
// what launches pods now and cmd/ binaries don't import one another.
// limits:  selection only — the backends live in internal/adapter/*, the port in
// internal/container.
package main

import (
	"os"
	"runtime"

	"github.com/flo-at/sindri/internal/adapter/container/applecontainer"
	"github.com/flo-at/sindri/internal/adapter/container/pod"
	"github.com/flo-at/sindri/internal/container"
)

// macDefaultRuntime is macOS's backend when SINDRI_RUNTIME is unset: Apple "container" gives one
// micro-VM per agent, so one crash or OOM cannot take the others down as the shared podman VM did.
// "podman" is the fallback. Linux always uses podman — Apple container needs macOS.
const macDefaultRuntime = "container"

// runtimeName resolves which container backend to use from the SINDRI_RUNTIME
// override and the host OS, as a plain name so it's testable without the host's
// actual GOOS. macOS defaults to Apple `container` (opt out with SINDRI_RUNTIME=
// podman); Linux always uses podman, since Apple `container` needs macOS.
func runtimeName(env, goos string) string {
	if goos != "darwin" {
		return "podman" // Apple `container` needs macOS; Linux always podman
	}
	switch env {
	case "podman":
		return "podman" // opt out of the macOS default
	case "container":
		return "container"
	default:
		return macDefaultRuntime // unset → the macOS default (Apple container)
	}
}

// chooseRuntime resolves the single container backend for this process, mapping
// the selected name to its adapter (the only place that imports them).
func chooseRuntime() container.Runtime {
	if runtimeName(os.Getenv("SINDRI_RUNTIME"), runtime.GOOS) == "container" {
		return applecontainer.Engine{}
	}
	return pod.Engine{} // podman: the default, and the only Linux runtime
}
