package main

import "testing"

// TestRuntimeNameDefaultsToPodman: podman is the default runtime everywhere when
// SINDRI_RUNTIME is unset — on Linux and macOS alike — so an existing install
// never silently flips to Apple `container` (which would break agents on a host
// without macOS 26 + the `container` tool). Apple container is opt-in on macOS.
func TestRuntimeNameDefaultsToPodman(t *testing.T) {
	cases := []struct{ env, goos, want string }{
		{"", "linux", "podman"},            // unconfigured on Linux → podman
		{"", "darwin", "podman"},           // unconfigured on macOS → podman (no breaking flip)
		{"container", "darwin", "container"}, // opt in to Apple container on macOS
		{"container", "linux", "podman"},   // container is macOS-only → falls back to podman
		{"podman", "darwin", "podman"},     // explicit podman on macOS
	}
	for _, c := range cases {
		if got := runtimeName(c.env, c.goos); got != c.want {
			t.Errorf("runtimeName(env=%q, goos=%q) = %q, want %q", c.env, c.goos, got, c.want)
		}
	}
}
