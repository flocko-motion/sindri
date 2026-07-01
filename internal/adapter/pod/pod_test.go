package pod

import (
	"io"
	"slices"
	"strings"
	"testing"
)

func TestCheckReportsMissingPodman(t *testing.T) {
	orig := Binary
	Binary = "podman-does-not-exist-xyz"
	t.Cleanup(func() { Binary = orig })
	err := Check(io.Discard)
	if err == nil {
		t.Fatal("expected an error when podman is not on PATH")
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("error should name the missing binary, got: %v", err)
	}
}

func TestRunArgsDeterministicAndComplete(t *testing.T) {
	got := RunArgs(RunOpts{
		Name:   "sindri-brokkr",
		Image:  "sindri-agent:test",
		Labels: map[string]string{"sindri.project": "/repo", "sindri.agent": "brokkr"},
		Env:    map[string]string{"B": "2", "A": "1"},
		Mounts: []Mount{
			{Host: "/h/ws", Container: "/workspace", Mode: "rw"},
			{Host: "/h/sock", Container: "/run/hub.sock", Mode: "rw"},
		},
		Workdir:    "/workspace",
		Entrypoint: []string{"bash", "-c", "sleep infinity"},
	})

	// Labels and env emitted in sorted key order → deterministic.
	want := []string{
		"run", "-d", "--replace", "--name", "sindri-brokkr", "--userns=" + UserNS,
		"--label", "sindri.agent=brokkr", "--label", "sindri.project=/repo",
		"-e", "A=1", "-e", "B=2",
		"-v", "/h/ws:/workspace:rw,z", "-v", "/h/sock:/run/hub.sock:rw,z",
		"-w", "/workspace",
		"sindri-agent:test", "bash", "-c", "sleep infinity",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("RunArgs mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestRunArgsDefaultMountMode(t *testing.T) {
	got := RunArgs(RunOpts{
		Name:   "c",
		Image:  "img",
		Mounts: []Mount{{Host: "/h", Container: "/c"}},
	})
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "/h:/c:rw,z") {
		t.Fatalf("default mount mode not rw: %q", joined)
	}
}
