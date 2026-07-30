package pod

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/container"
)

func TestCheckReportsMissingPodman(t *testing.T) {
	orig := Binary
	Binary = "podman-does-not-exist-xyz"
	t.Cleanup(func() { Binary = orig })
	err := Engine{}.Check(io.Discard)
	if err == nil {
		t.Fatal("expected an error when podman is not on PATH")
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("error should name the missing binary, got: %v", err)
	}
}

func TestRunArgsDeterministicAndComplete(t *testing.T) {
	got := RunArgs(container.RunOpts{
		Name:   "sindri-brokkr",
		Image:  "sindri-agent:test",
		Labels: map[string]string{"sindri.project": "/repo", "sindri.agent": "brokkr"},
		Env:    map[string]string{"B": "2", "A": "1"},
		Mounts: []container.Mount{
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
	got := RunArgs(container.RunOpts{
		Name:   "c",
		Image:  "img",
		Mounts: []container.Mount{{Host: "/h", Container: "/c"}},
	})
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "/h:/c:rw,z") {
		t.Fatalf("default mount mode not rw: %q", joined)
	}
}

func TestAttachCmdForwardsTerminalEnv(t *testing.T) {
	// The interactive attach must carry the caller's TERM/COLORTERM into the pod, or
	// the tmux attach client comes up with an empty TERM and renders scrambled. It
	// goes BEFORE the container name (an `-e` after the name would be an arg to the
	// in-pod command, not to `exec`).
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")

	args := Engine{}.AttachCmd("pod1", "tmux", "attach").Args
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-e TERM=xterm-256color") || !strings.Contains(joined, "-e COLORTERM=truecolor") {
		t.Fatalf("attach must forward TERM/COLORTERM, got: %q", joined)
	}
	name, cmd := slices.Index(args, "pod1"), slices.Index(args, "tmux")
	if term := slices.Index(args, "TERM=xterm-256color"); term > name || name > cmd {
		t.Fatalf("env must precede the pod name, which must precede the command: %q", joined)
	}

	// With no TERM on the host there's nothing to forward — the pod keeps its own
	// COLORTERM default rather than being handed an empty value.
	t.Setenv("TERM", "")
	t.Setenv("COLORTERM", "")
	if joined := strings.Join(Engine{}.AttachCmd("pod1", "tmux").Args, " "); strings.Contains(joined, "TERM=") {
		t.Fatalf("empty host TERM must not be forwarded, got: %q", joined)
	}
}

func TestBuildFailureDetail(t *testing.T) {
	// A connection failure (the macOS "machine not started" case) surfaces the
	// message AND the podman-machine hint.
	got := buildFailureDetail("Error: Cannot connect to Podman socket")
	if !strings.Contains(got, "Cannot connect") || !strings.Contains(got, "podman machine start") {
		t.Errorf("connection failure should surface the error + hint, got:\n%s", got)
	}
	// Empty output still yields the hint (better than nothing).
	if got := buildFailureDetail(""); !strings.Contains(got, "podman machine") {
		t.Errorf("empty output should fall back to the hint, got: %q", got)
	}
	// An ordinary build error (no connection signal) is surfaced without the hint.
	got = buildFailureDetail("Step 3/9: RUN apt-get install foo\nE: Unable to locate package foo")
	if !strings.Contains(got, "Unable to locate package foo") {
		t.Errorf("build error should be surfaced, got:\n%s", got)
	}
	if strings.Contains(got, "podman machine") {
		t.Errorf("a normal build error must not get the machine hint, got:\n%s", got)
	}
	// Long output is tailed to the last lines (where the error lands).
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("layer line\n")
	}
	b.WriteString("Error: final failure line")
	got = buildFailureDetail(b.String())
	if !strings.Contains(got, "final failure line") {
		t.Errorf("tail must include the final error line, got:\n%s", got)
	}
	if strings.Count(got, "\n") > 12 {
		t.Errorf("tail should be bounded (~12 lines), got %d lines", strings.Count(got, "\n"))
	}
}
