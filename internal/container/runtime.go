// package: container / runtime
// type:    logic (the container-runtime PORT — hexagonal abstraction)
// job:     declare the runtime contract the hub uses to run agent pods (run, exec,
//          attach, liveness, logs, remove, orphan-list, pre-flight, image build)
//          plus the shared value types, and hold the one backend the process is
//          wired to. This is the abstraction; adapters implement it.
// limits:  no CLI here — the podman and apple-container implementations live in
//          internal/adapter/*, which depend on this package, never the reverse.
//          The composition root (cmd/sindri) selects a backend via Use.
package container

import (
	"context"
	"errors"
	"io"
	"os/exec"
)

// Mount is one bind mount into an agent pod.
type Mount struct {
	Host      string
	Container string
	Mode      string // "ro" | "rw"
}

// RunOpts configures a detached agent pod.
type RunOpts struct {
	Name       string
	Image      string
	Labels     map[string]string
	Env        map[string]string
	Mounts     []Mount
	Workdir    string
	Entrypoint []string
}

// Runtime is the port: everything the hub needs from a container backend. Podman
// (internal/adapter/pod) and Apple container (internal/adapter/applecontainer) each
// implement it; the hub/CLI/TUI depend only on this interface.
type Runtime interface {
	Run(o RunOpts) error
	Exec(name string, args ...string) ([]byte, error)
	ExecContext(ctx context.Context, name string, args ...string) ([]byte, error)
	ExecInteractive(name string, args ...string) error
	// AttachCmd returns (without running) the interactive exec command, so a caller
	// that drives its own terminal handoff (the TUI) can manage it.
	AttachCmd(name string, args ...string) *exec.Cmd
	Running(name string) bool
	RunningContext(ctx context.Context, name string) bool
	Logs(name string, tail int) string
	Info(name string) string
	Rm(name string) error
	ListByLabelContext(ctx context.Context, label, value string) ([]string, error)
	Check(w io.Writer) error
	Healthy() (ok bool, hint string)
	EnsureImage(root string, out io.Writer) error
}

// active is the backend this process runs against, wired once at startup by the
// composition root via Use. Defaults to a no-op backend (reports "unavailable"
// rather than nil-panicking) so the port is safe before Use — the state a
// worker-only process or a test sees when no backend is wired.
var active Runtime = noop{}

// Use selects the container backend for this process. Called once at startup.
func Use(r Runtime) { active = r }

// errNoRuntime is returned by the no-op backend's mutating ops.
var errNoRuntime = errors.New("no container runtime configured")

// noop is the default backend until Use wires a real one: reads report nothing
// present; mutations error. It keeps the port panic-free when unwired.
type noop struct{}

func (noop) Run(RunOpts) error                                          { return errNoRuntime }
func (noop) Exec(string, ...string) ([]byte, error)                     { return nil, errNoRuntime }
func (noop) ExecContext(context.Context, string, ...string) ([]byte, error) { return nil, errNoRuntime }
func (noop) ExecInteractive(string, ...string) error                    { return errNoRuntime }
func (noop) AttachCmd(string, ...string) *exec.Cmd                      { return exec.Command("true") }
func (noop) Running(string) bool                                        { return false }
func (noop) RunningContext(context.Context, string) bool                { return false }
func (noop) Logs(string, int) string                                    { return "" }
func (noop) Info(string) string                                         { return "" }
func (noop) Rm(string) error                                            { return errNoRuntime }
func (noop) ListByLabelContext(context.Context, string, string) ([]string, error) { return nil, nil }
func (noop) Check(io.Writer) error                                      { return errNoRuntime }
func (noop) Healthy() (bool, string)                                    { return false, "no container runtime configured" }
func (noop) EnsureImage(string, io.Writer) error                        { return errNoRuntime }

// --- package façade: the core calls these; they dispatch to the wired backend ---

// Run launches a detached agent pod on the wired backend.
func Run(o RunOpts) error { return active.Run(o) }

// Exec runs a command in a pod and returns its combined output.
func Exec(name string, args ...string) ([]byte, error) { return active.Exec(name, args...) }

// ExecContext is Exec bounded by ctx.
func ExecContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	return active.ExecContext(ctx, name, args...)
}

// ExecInteractive runs a command wired to the caller's TTY (the human dial-in).
func ExecInteractive(name string, args ...string) error { return active.ExecInteractive(name, args...) }

// AttachCmd returns (without running) the interactive exec command.
func AttachCmd(name string, args ...string) *exec.Cmd { return active.AttachCmd(name, args...) }

// Running reports whether a pod is running.
func Running(name string) bool { return active.Running(name) }

// RunningContext is Running bounded by ctx.
func RunningContext(ctx context.Context, name string) bool {
	return active.RunningContext(ctx, name)
}

// Logs returns the last `tail` lines of a pod's output.
func Logs(name string, tail int) string { return active.Logs(name, tail) }

// Info returns a short summary of a pod.
func Info(name string) string { return active.Info(name) }

// Rm force-removes a pod.
func Rm(name string) error { return active.Rm(name) }

// ListByLabelContext lists pods carrying label=value (orphan detection).
func ListByLabelContext(ctx context.Context, label, value string) ([]string, error) {
	return active.ListByLabelContext(ctx, label, value)
}

// Check pre-flights the runtime (installed + reachable), auto-starting where it can.
func Check(w io.Writer) error { return active.Check(w) }

// Healthy is a fast, time-bounded reachability probe.
func Healthy() (ok bool, hint string) { return active.Healthy() }

// EnsureImage builds the agent image if the recipe is stale.
func EnsureImage(root string, out io.Writer) error { return active.EnsureImage(root, out) }
