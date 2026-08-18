// package: container / runtime
// type:    logic (the container-runtime PORT — hexagonal abstraction)
// job:     declare the runtime contract the hub uses to run agent pods (run, exec,
// attach, liveness, logs, remove, orphan-list, pre-flight, image build)
// plus the shared value types, and hold the one backend the process is
// wired to. This is the abstraction; adapters implement it.
// limits:  no CLI here — the podman and apple-container implementations live in
// internal/adapter/*, which depend on this package, never the reverse.
// The composition root (cmd/sindri) selects a backend via Use.
package container

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// TermEnvArgs forwards TERM/COLORTERM into an interactive `exec -it`: exec drops the
// host env, and tmux without TERM renders scrambled. Only set variables are passed.
// Interactive attach only — the parsed Exec must not carry TERM.
func TermEnvArgs() []string {
	var args []string
	for _, k := range []string{"TERM", "COLORTERM"} {
		if v := os.Getenv(k); v != "" {
			args = append(args, "-e", k+"="+v)
		}
	}
	return args
}

// Mount is one bind mount into an agent pod.
type Mount struct {
	Host      string
	Container string
	Mode      string // "ro" | "rw"
}

// Usage snapshots a running pod's resources; a zero field means unreported.
type Usage struct {
	MemoryUsageBytes int64
	MemoryLimitBytes int64
}

// The two things UsedBytes can count, which is the difference between the backends: containers
// sharing the host kernel take memory as they use it, a micro-VM takes its whole limit when it
// starts. Named so a reader of the figure knows which question it answers.
const (
	BasisInUse    = "in use"
	BasisReserved = "reserved"
)

// Capacity is the fleet's memory situation: what running pods cost the machine (UsedBytes) against
// the ceiling they draw from (TotalBytes), in bytes, with Basis naming what Used counts. A zero
// TotalBytes means the backend could not say.
type Capacity struct {
	UsedBytes  int64
	TotalBytes int64
	Basis      string // BasisInUse | BasisReserved
}

// NetChannel is the macOS agent TCP channel: a unix socket can't cross the VM
// boundary, so the hub binds at BindAddr and pods dial DialHost (SINDRI_HUB_ADDR).
// Per-backend: podman forwards host.containers.internal to loopback; apple-container
// pods reach the host only at the gateway IP, which must also be the bind address.
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

// Runtime is the port: everything the hub needs from a container backend.
type Runtime interface {
	// Name identifies the backend for humans (e.g. "podman", "apple container").
	Name() string
	// DefaultMemory is the limit an agent gets when none is configured. The backend answers it
	// because the right number is a property of how it runs a container: a shared-kernel container
	// takes what it uses from the host, a micro-VM reserves its whole limit up front.
	DefaultMemory() string
	Run(o RunOpts) error
	Exec(name string, args ...string) ([]byte, error)
	ExecContext(ctx context.Context, name string, args ...string) ([]byte, error)
	ExecInteractive(name string, args ...string) error
	// AttachCmd returns the interactive exec command unrun, for callers (the TUI)
	// that drive their own terminal handoff.
	AttachCmd(name string, args ...string) *exec.Cmd
	Running(name string) bool
	RunningContext(ctx context.Context, name string) bool
	// Diagnose accounts for what the running probe observes, so a "not running"
	// verdict is explainable rather than a silent false.
	Diagnose(ctx context.Context, name string) string
	// Stats snapshots memory usage vs limit, to show how close a pod is to its ceiling.
	Stats(ctx context.Context, name string) (Usage, error)
	// MemoryCapacity reports the whole fleet's memory against the ceiling it draws from, so a
	// caller can tell whether another agent fits. The backend answers for the reason DefaultMemory
	// is its to state: a shared-kernel container draws host memory as it uses it and may be
	// overcommitted, a micro-VM reserves its limit up front and refuses to start when the
	// reservation does not fit, and podman on macOS is bounded by its VM rather than by the Mac.
	MemoryCapacity(ctx context.Context) (Capacity, error)
	// AgentChannel reports the macOS TCP channel; backend-specific networking.
	AgentChannel() (NetChannel, error)
	Logs(name string, tail int) string
	Info(name string) string
	Rm(name string) error
	ListByLabelContext(ctx context.Context, label, value string) ([]string, error)
	// Check pre-flights the runtime, narrating to w. There is no separate reachability probe: the
	// hub learns that from the pod listing its liveness sweep already takes, so a caller asking
	// again paid seconds for an answer it had (-> hub/watchdog.runtimeHint).
	Check(w io.Writer) error
	EnsureImage(root, containerfile string, out io.Writer) (string, error)
	// RebuildImage rebuilds re-pulling the base, to pick up one the cache keeps stale.
	RebuildImage(root, containerfile string, out io.Writer) (string, error)
}

// active is wired once at startup via Use; the no-op default keeps the port
// panic-free for worker-only processes and tests that never wire a backend.
var active Runtime = noop{}

// Use selects the container backend for this process. Called once at startup.
func Use(r Runtime) { active = r }

// UseDefault restores the unwired default — reads report nothing present, mutations error. What a
// caller that wired a backend for a moment puts back, so what follows starts where an unwired
// process does: a partial stand-in left behind panics the first time anything reaches a method it
// never implemented.
func UseDefault() { active = noop{} }

// errNoRuntime is returned by the no-op backend's mutating ops.
var errNoRuntime = errors.New("no container runtime configured")

// noop is the default until Use: reads report nothing present, mutations error.
type noop struct{}

func (noop) Name() string                                                   { return "none (no runtime configured)" }
func (noop) DefaultMemory() string                                          { return "" }
func (noop) Run(RunOpts) error                                              { return errNoRuntime }
func (noop) Exec(string, ...string) ([]byte, error)                         { return nil, errNoRuntime }
func (noop) ExecContext(context.Context, string, ...string) ([]byte, error) { return nil, errNoRuntime }
func (noop) ExecInteractive(string, ...string) error                        { return errNoRuntime }
func (noop) AttachCmd(string, ...string) *exec.Cmd                          { return exec.Command("true") }
func (noop) Running(string) bool                                            { return false }
func (noop) RunningContext(context.Context, string) bool                    { return false }
func (noop) Diagnose(context.Context, string) string                        { return "no container runtime configured" }
func (noop) Stats(context.Context, string) (Usage, error)                   { return Usage{}, errNoRuntime }
func (noop) MemoryCapacity(context.Context) (Capacity, error)               { return Capacity{}, errNoRuntime }

// AgentChannel returns the podman-style loopback default, not an error: an unwired
// backend has a legitimate config, and production always wires a real one via Use.
func (noop) AgentChannel() (NetChannel, error) {
	return NetChannel{BindAddr: "127.0.0.1", DialHost: "host.containers.internal"}, nil
}
func (noop) Logs(string, int) string                                              { return "" }
func (noop) Info(string) string                                                   { return "" }
func (noop) Rm(string) error                                                      { return errNoRuntime }
func (noop) ListByLabelContext(context.Context, string, string) ([]string, error) { return nil, nil }
func (noop) Check(io.Writer) error                                                { return errNoRuntime }
func (noop) EnsureImage(string, string, io.Writer) (string, error)                { return "", errNoRuntime }
func (noop) RebuildImage(string, string, io.Writer) (string, error)               { return "", errNoRuntime }

// --- package façade: dispatches to the wired backend ---

// Name identifies the wired backend for humans (e.g. "podman", "apple container").
func Name() string { return active.Name() }

// DefaultMemory is the wired backend's per-agent memory default.
func DefaultMemory() string { return active.DefaultMemory() }

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

// Diagnose explains what the running probe observes, so "not running" isn't a shrug.
func Diagnose(ctx context.Context, name string) string { return active.Diagnose(ctx, name) }

// Stats returns a point-in-time resource snapshot (memory usage vs limit) for a pod.
func Stats(ctx context.Context, name string) (Usage, error) { return active.Stats(ctx, name) }

// MemoryCapacity reports the fleet's memory against the ceiling the wired backend draws from.
func MemoryCapacity(ctx context.Context) (Capacity, error) { return active.MemoryCapacity(ctx) }

// AgentChannel reports how the macOS agent TCP channel is bound and addressed.
func AgentChannel() (NetChannel, error) { return active.AgentChannel() }

// Logs returns the last `tail` lines of a pod's output.
func Logs(name string, tail int) string { return active.Logs(name, tail) }

// Info returns a short summary of a pod.
func Info(name string) string { return active.Info(name) }

// Rm force-removes a pod.
func Rm(name string) error { return active.Rm(name) }

// ListTTL bounds ListByLabelCached staleness: short enough that a board read seconds
// later is current, long enough that a burst (poll tick + refetch + keystrokes) costs one.
const ListTTL = 1500 * time.Millisecond

var listMemo struct {
	mu   sync.Mutex
	at   time.Time
	key  string
	pods []string
	err  error
}

// ListByLabelCached lists pods carrying label=value (empty value: any), memoized for
// ListTTL. Every hot-path listing goes through it: each call is a process spawn, and
// 24 concurrently measure ~3.2s against ~0.2s for one — the cost is the spawn count.
func ListByLabelCached(ctx context.Context, label, value string) ([]string, error) {
	listMemo.mu.Lock()
	defer listMemo.mu.Unlock()
	key := label + "=" + value
	if listMemo.key == key && !listMemo.at.IsZero() && time.Since(listMemo.at) < ListTTL {
		return listMemo.pods, listMemo.err
	}
	pods, err := active.ListByLabelContext(ctx, label, value)
	listMemo.at, listMemo.key, listMemo.pods, listMemo.err = time.Now(), key, pods, err
	return pods, err
}

// ListByLabelFresh lists without consulting the memo, and primes it with the result. For a caller
// whose question is about NOW: a listing taken before a container was created does not mention it,
// and absence from a listing is the evidence an agent is gone. Priming rather than bypassing keeps
// the following board reads on this newer answer instead of the one it just overtook.
func ListByLabelFresh(ctx context.Context, label, value string) ([]string, error) {
	pods, err := active.ListByLabelContext(ctx, label, value)
	listMemo.mu.Lock()
	listMemo.at, listMemo.key, listMemo.pods, listMemo.err = time.Now(), label+"="+value, pods, err
	listMemo.mu.Unlock()
	return pods, err
}

// Check pre-flights the runtime (installed + reachable), auto-starting where it can.
func Check(w io.Writer) error { return active.Check(w) }

// EnsureImage builds the agent image if the recipe is stale, returning the reference.
func EnsureImage(root, containerfile string, out io.Writer) (string, error) {
	return active.EnsureImage(root, containerfile, out)
}

// RebuildImage rebuilds the agent image, re-pulling the base.
func RebuildImage(root, containerfile string, out io.Writer) (string, error) {
	return active.RebuildImage(root, containerfile, out)
}
