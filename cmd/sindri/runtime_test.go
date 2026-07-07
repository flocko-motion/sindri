package main

import (
	"runtime"
	"testing"
)

// TestRuntimeNameDefaultsToPodman: podman is the default runtime everywhere when
// SINDRI_RUNTIME is unset — on Linux and macOS alike — so an existing install
// never silently flips to Apple `container` (which would break agents on a host
// without macOS 26 + the `container` tool). Apple container is opt-in on macOS.
// runtimeName takes goos as a parameter so both platforms are covered regardless
// of the host the test runs on.
func TestRuntimeNameDefaultsToPodman(t *testing.T) {
	cases := []struct{ env, goos, want string }{
		{"", "linux", "podman"},              // unconfigured on Linux → podman
		{"", "darwin", "podman"},             // unconfigured on macOS → podman (no breaking flip)
		{"container", "darwin", "container"}, // opt in to Apple container on macOS
		{"container", "linux", "podman"},     // container is macOS-only → falls back to podman
		{"podman", "darwin", "podman"},       // explicit podman on macOS
	}
	for _, c := range cases {
		if got := runtimeName(c.env, c.goos); got != c.want {
			t.Errorf("runtimeName(env=%q, goos=%q) = %q, want %q", c.env, c.goos, got, c.want)
		}
	}
}

// TestChooseRuntimeDefaultIsPodman: the wired backend (what main.go injects) is
// podman when SINDRI_RUNTIME is unset. chooseRuntime reads the real GOOS, so this
// asserts the running platform directly; the darwin default is covered above via
// runtimeName. On Linux even SINDRI_RUNTIME=container stays podman (macOS-only).
func TestChooseRuntimeDefaultIsPodman(t *testing.T) {
	t.Setenv("SINDRI_RUNTIME", "")
	if got := chooseRuntime().Name(); got != "podman" {
		t.Fatalf("chooseRuntime().Name() with SINDRI_RUNTIME unset = %q, want podman", got)
	}
	if runtime.GOOS != "darwin" { // container is macOS-only; off macOS it must fall back to podman
		t.Setenv("SINDRI_RUNTIME", "container")
		if got := chooseRuntime().Name(); got != "podman" {
			t.Fatalf("chooseRuntime().Name() with container off macOS = %q, want podman (fallback)", got)
		}
	}
}
