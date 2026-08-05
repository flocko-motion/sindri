package main

import (
	"runtime"
	"testing"
)

// TestRuntimeNameDefaults: macOS defaults to Apple `container` (one micro-VM per
// agent — the isolation sindri wants), opt out with SINDRI_RUNTIME=podman; Linux
// always uses podman since Apple `container` needs macOS. runtimeName takes goos as
// a parameter so both platforms are covered regardless of the host the test runs on.
func TestRuntimeNameDefaults(t *testing.T) {
	cases := []struct{ env, goos, want string }{
		{"", "darwin", "container"},          // unconfigured on macOS → Apple container (the default)
		{"", "linux", "podman"},              // unconfigured on Linux → podman
		{"podman", "darwin", "podman"},       // opt out of the macOS default
		{"container", "darwin", "container"}, // explicit Apple container on macOS
		{"container", "linux", "podman"},     // container is macOS-only → falls back to podman
		{"podman", "linux", "podman"},        // explicit podman on Linux
	}
	for _, c := range cases {
		if got := runtimeName(c.env, c.goos); got != c.want {
			t.Errorf("runtimeName(env=%q, goos=%q) = %q, want %q", c.env, c.goos, got, c.want)
		}
	}
}

// TestChooseRuntimeDefault: the wired backend (what main.go injects) with
// SINDRI_RUNTIME unset — Apple container on macOS, podman elsewhere. chooseRuntime
// reads the real GOOS, so this asserts the running platform directly.
func TestChooseRuntimeDefault(t *testing.T) {
	t.Setenv("SINDRI_RUNTIME", "")
	want := "podman"
	if runtime.GOOS == "darwin" {
		want = "apple container"
	}
	if got := chooseRuntime().Name(); got != want {
		t.Fatalf("chooseRuntime().Name() unset on %s = %q, want %q", runtime.GOOS, got, want)
	}
	if runtime.GOOS == "darwin" { // opt out of the macOS default
		t.Setenv("SINDRI_RUNTIME", "podman")
		if got := chooseRuntime().Name(); got != "podman" {
			t.Fatalf("SINDRI_RUNTIME=podman on macOS = %q, want podman", got)
		}
	} else { // container is macOS-only; off macOS it must fall back to podman
		t.Setenv("SINDRI_RUNTIME", "container")
		if got := chooseRuntime().Name(); got != "podman" {
			t.Fatalf("SINDRI_RUNTIME=container off macOS = %q, want podman (fallback)", got)
		}
	}
}
