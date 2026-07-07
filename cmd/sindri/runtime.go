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
// unset. Flip this one constant to change the macOS default:
//
//	"container" — Apple `container`, one micro-VM per agent (isolated failures)
//	"podman"    — the shared podman VM
//
// (Linux always uses podman.)
const macDefaultRuntime = "container"

// chooseRuntime resolves the single container backend for this process. SINDRI_RUNTIME
// overrides; otherwise the platform default applies (Linux → podman always; macOS →
// macDefaultRuntime). Apple `container` off macOS falls back to podman.
func chooseRuntime() container.Runtime {
	name := os.Getenv("SINDRI_RUNTIME")
	if name == "" {
		if runtime.GOOS == "darwin" {
			name = macDefaultRuntime
		} else {
			name = "podman"
		}
	}
	if name == "container" && runtime.GOOS == "darwin" {
		return applecontainer.Engine{}
	}
	return pod.Engine{} // podman: the default, and the only Linux runtime
}
