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

// Usage is a point-in-time resource snapshot of a running pod. A zero field means
// the backend didn't report that metric.
type Usage struct {
	MemoryUsageBytes int64
	MemoryLimitBytes int64
}

// NetChannel describes how the macOS agent TCP channel is reached — on macOS a
// bind-mounted unix socket can't cross the VM boundary, so the hub serves agents
// over TCP: it binds the listener at BindAddr, and a pod addresses the host at
// DialHost (handed to the pod as SINDRI_HUB_ADDR). These differ per backend: podman
// resolves the magic name host.containers.internal to a forwarded loopback (bind
// 127.0.0.1), while apple-container pods reach the host directly at the network
// gateway, so the hub must both bind and advertise that gateway IP. Obtained via
// the runtime's AgentChannel method.
type NetChannel struct {
	BindAddr string
	DialHost string
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
	Memory     string // memory limit passed to the runtime (-m), e.g. "4g"; "" = runtime default
}

// Runtime is the port: everything the hub needs from a container backend. Podman
// (internal/adapter/pod) and Apple container (internal/adapter/applecontainer) each
// implement it; the hub/CLI/TUI depend only on this interface.
type Runtime interface {
	// Name identifies the backend for humans (e.g. "podman", "apple container").
	Name() string
	Run(o RunOpts) error
	Exec(name string, args ...string) ([]byte, error)
	ExecContext(ctx context.Context, name string, args ...string) ([]byte, error)
	ExecInteractive(name string, args ...string) error
	// AttachCmd returns (without running) the interactive exec command, so a caller
	// that drives its own terminal handoff (the TUI) can manage it.
	AttachCmd(name string, args ...string) *exec.Cmd
	Running(name string) bool
	RunningContext(ctx context.Context, name string) bool
	// Diagnose returns a one-line, human-readable account of what the running
	// probe actually observes for name (raw command result, stderr, parsed state)
	// — so a "not running" verdict is explainable, not a silent false.
	Diagnose(ctx context.Context, name string) string
	// Stats returns a point-in-time resource snapshot (memory usage vs limit) for a
	// running pod, so the human can see how close an agent's VM is to its ceiling.
	Stats(ctx context.Context, name string) (Usage, error)
	// AgentChannel reports how the macOS agent TCP channel is bound and addressed
	// (see AgentChannel). Backend-specific because pod↔host networking differs.
	AgentChannel() (NetChannel, error)
	Logs(name string, tail int) string
	Info(name string) string
	Rm(name string) error
	ListByLabelContext(ctx context.Context, label, value string) ([]string, error)
	Check(w io.Writer) error
	Healthy() (ok bool, hint string)
	EnsureImage(root, containerfile string, out io.Writer) (string, error)
	// RebuildImage forces a rebuild of the agent image, re-pulling the base — for
	// picking up a newer base (e.g. a new Go) the cache would otherwise keep stale.
	RebuildImage(root, containerfile string, out io.Writer) (string, error)
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

func (noop) Name() string                                               { return "none (no runtime configured)" }
func (noop) Run(RunOpts) error                                          { return errNoRuntime }
func (noop) Exec(string, ...string) ([]byte, error)                     { return nil, errNoRuntime }
func (noop) ExecContext(context.Context, string, ...string) ([]byte, error) { return nil, errNoRuntime }
func (noop) ExecInteractive(string, ...string) error                    { return errNoRuntime }
func (noop) AttachCmd(string, ...string) *exec.Cmd                      { return exec.Command("true") }
func (noop) Running(string) bool                                        { return false }
func (noop) RunningContext(context.Context, string) bool                { return false }
func (noop) Diagnose(context.Context, string) string                    { return "no container runtime configured" }
func (noop) Stats(context.Context, string) (Usage, error)               { return Usage{}, errNoRuntime }

// AgentChannel: the historical loopback default (podman-style). Not an error — a
// legitimate default config for an unwired backend; production always wires a real
// one via Use, whose AgentChannel reflects that backend's actual networking.
func (noop) AgentChannel() (NetChannel, error) {
	return NetChannel{BindAddr: "127.0.0.1", DialHost: "host.containers.internal"}, nil
}
func (noop) Logs(string, int) string                                    { return "" }
func (noop) Info(string) string                                         { return "" }
func (noop) Rm(string) error                                            { return errNoRuntime }
func (noop) ListByLabelContext(context.Context, string, string) ([]string, error) { return nil, nil }
func (noop) Check(io.Writer) error                                      { return errNoRuntime }
func (noop) Healthy() (bool, string)                                    { return false, "no container runtime configured" }
func (noop) EnsureImage(string, string, io.Writer) (string, error)      { return "", errNoRuntime }
func (noop) RebuildImage(string, string, io.Writer) (string, error)     { return "", errNoRuntime }

// --- package façade: the core calls these; they dispatch to the wired backend ---

// Name identifies the wired backend for humans (e.g. "podman", "apple container").
func Name() string { return active.Name() }

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

// Diagnose returns a one-line account of what the running probe observes for name,
// so a "not running" verdict can be explained instead of shrugged at.
func Diagnose(ctx context.Context, name string) string { return active.Diagnose(ctx, name) }

// Stats returns a point-in-time resource snapshot (memory usage vs limit) for a pod.
func Stats(ctx context.Context, name string) (Usage, error) { return active.Stats(ctx, name) }

// AgentChannel reports how the macOS agent TCP channel is bound and addressed.
func AgentChannel() (NetChannel, error) { return active.AgentChannel() }

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

// EnsureImage builds the agent image if the recipe is stale, returning the image
// reference to run (default or a custom per-recipe tag).
func EnsureImage(root, containerfile string, out io.Writer) (string, error) {
	return active.EnsureImage(root, containerfile, out)
}

// RebuildImage forces a rebuild of the agent image (re-pulling the base) and returns
// the image reference to run.
func RebuildImage(root, containerfile string, out io.Writer) (string, error) {
	return active.RebuildImage(root, containerfile, out)
}
