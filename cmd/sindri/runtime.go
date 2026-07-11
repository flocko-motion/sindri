// package: main (sindri) / runtime
// type:    composition root (wires the container backend)
// job:     pick the ONE container runtime this process uses and inject it into the
//          container port. This is the only place that imports the adapters.
// limits:  selection only — the backends live in internal/adapter/*, the port in
//          internal/container.
package main

import (
	"os"
	"runtime"

	"github.com/flo-at/sindri/internal/adapter/container/applecontainer"
	"github.com/flo-at/sindri/internal/adapter/container/pod"
	"github.com/flo-at/sindri/internal/container"
)

// macDefaultRuntime is the container backend used on macOS when SINDRI_RUNTIME is
// unset. macOS defaults to Apple `container` — one micro-VM per agent, so one
// agent's crash or OOM can't take the others down. That isolation is the whole
// reason sindri moved off the shared podman VM on macOS; podman is the fallback,
// opt in with SINDRI_RUNTIME=podman. Flip this one constant to change the default:
//
//	"container" — Apple `container`, one micro-VM per agent (macOS default)
//	"podman"    — the shared podman VM
//
// (Linux always uses podman — Apple `container` needs macOS.)
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
