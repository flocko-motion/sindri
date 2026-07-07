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

	"github.com/flo-at/sindri/internal/adapter/applecontainer"
	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/container"
)

// macDefaultRuntime is the container backend used on macOS when SINDRI_RUNTIME is
// unset. Podman is the default everywhere — an existing macOS install keeps its
// podman backend (no breaking flip); Apple `container` is opt-in via
// SINDRI_RUNTIME=container. Flip this one constant to change the macOS default:
//
//	"podman"    — the shared podman VM (default)
//	"container" — Apple `container`, one micro-VM per agent (isolated failures)
//
// (Linux always uses podman.)
const macDefaultRuntime = "podman"

// runtimeName resolves which container backend to use from the SINDRI_RUNTIME
// override and the host OS, as a plain name so it's testable without the host's
// actual GOOS. Podman is the default everywhere (Linux always); Apple `container`
// is opt-in on macOS, and requested off macOS it falls back to podman.
func runtimeName(env, goos string) string {
	if env == "" {
		if goos == "darwin" {
			return macDefaultRuntime
		}
		return "podman"
	}
	if env == "container" && goos == "darwin" {
		return "container"
	}
	return "podman"
}

// chooseRuntime resolves the single container backend for this process, mapping
// the selected name to its adapter (the only place that imports them).
func chooseRuntime() container.Runtime {
	if runtimeName(os.Getenv("SINDRI_RUNTIME"), runtime.GOOS) == "container" {
		return applecontainer.Engine{}
	}
	return pod.Engine{} // podman: the default, and the only Linux runtime
}
