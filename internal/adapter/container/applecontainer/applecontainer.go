// package: adapter/applecontainer / applecontainer
// type:    adapter (external tool: Apple `container`) — implements container.Runtime
// job:     the Apple-`container` backend for the container-runtime port (macOS 26):
// each agent pod is its OWN micro-VM, so one agent's crash/OOM can't take
// down the others. Maps run/exec/attach/liveness/logs/remove/orphan-list/
// image-build onto the `container` CLI.
// limits:  implements container.Runtime; wired in at the composition root. macOS
// only (needs the `container` service + a Linux kernel per micro-VM).
package applecontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/container"
)

// Binary is the Apple container executable.
var Binary = "container"

// Engine is the Apple-`container` implementation of container.Runtime.
type Engine struct{}

// Name identifies this backend for humans.
func (Engine) Name() string { return "apple container" }

// DefaultMemory: a micro-VM takes its limit from the host as a RESERVATION, used or not, so the
// number is what each agent costs the machine simply by running. It stays modest for that reason —
// on a small Mac a few agents at once must not crowd it out — and is raised per agent when needed.
func (Engine) DefaultMemory() string { return "2g" }

// AgentChannel advertises the gateway (micro-VMs have no host.containers.internal),
// read from the runtime, never assumed to be .1, and never defaulted on failure.
func (Engine) AgentChannel() (container.NetChannel, error) {
	out, err := exec.Command(Binary, "network", "inspect", "default").Output()
	if err != nil {
		return container.NetChannel{}, fmt.Errorf("container network inspect default: %w", err)
	}
	var nets []struct {
		Status struct {
			IPv4Gateway string `json:"ipv4Gateway"`
		} `json:"status"`
	}
	if e := json.Unmarshal(out, &nets); e != nil {
		return container.NetChannel{}, fmt.Errorf("parse container network inspect default: %w", e)
	}
	if len(nets) == 0 || nets[0].Status.IPv4Gateway == "" {
		return container.NetChannel{}, fmt.Errorf("container network 'default' reports no ipv4Gateway")
	}
	gw := nets[0].Status.IPv4Gateway
	return container.NetChannel{BindAddr: gw, DialHost: gw}, nil
}

// inspectEntry must stay faithful to `container inspect`: configuration.image is an
// OBJECT, and typing it as a string failed the unmarshal, so live pods read as down.
type inspectEntry struct {
	ID     string `json:"id"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
	Configuration struct {
		ID    string `json:"id"`
		Image struct {
			Reference string `json:"reference"`
		} `json:"image"`
		Labels    map[string]string `json:"labels"`
		Resources struct {
			CPUs          int   `json:"cpus"`
			MemoryInBytes int64 `json:"memoryInBytes"`
		} `json:"resources"`
		Platform struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		} `json:"platform"`
	} `json:"configuration"`
}

// hostPID finds the micro-VM's macOS runtime process by matching the
// `container-runtime-linux … --uuid <name>` argv. "" if not found.
func hostPID(name string) string {
	out, err := exec.Command("pgrep", "-fl", "container-runtime-linux").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(line, "--uuid "+name) {
			if f := strings.Fields(line); len(f) > 0 {
				return f[0]
			}
		}
	}
	return ""
}

// parseInspect unmarshals `container inspect`/`ls` JSON. A shape mismatch is an adapter
// bug, never a "not present", so it is logged loudly rather than swallowed as false.
func parseInspect(what string, raw []byte) ([]inspectEntry, error) {
	var entries []inspectEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		log.Printf("applecontainer: %s returned JSON we can't parse (adapter bug — schema drift?): %v", what, err)
		return nil, err
	}
	return entries, nil
}

// runArgs builds `container run -d …`: no `--userns` (micro-VMs map mounts themselves)
// and no `--replace` (Run removes any stale first).
func runArgs(o container.RunOpts) []string {
	args := []string{"run", "-d", "--name", o.Name}
	if o.Memory != "" {
		args = append(args, "-m", o.Memory)
	}
	for _, k := range sortedKeys(o.Labels) {
		args = append(args, "-l", k+"="+o.Labels[k])
	}
	for _, k := range sortedKeys(o.Env) {
		args = append(args, "-e", k+"="+o.Env[k])
	}
	for _, m := range o.Mounts {
		v := m.Host + ":" + m.Container
		if m.Mode == "ro" {
			v += ":ro"
		}
		args = append(args, "-v", v)
	}
	if o.Workdir != "" {
		args = append(args, "-w", o.Workdir)
	}
	args = append(args, o.Image)
	args = append(args, o.Entrypoint...)
	return args
}

// Run launches a detached micro-VM pod (no --replace, so it clears any stale first).
func (Engine) Run(o container.RunOpts) error {
	_ = exec.Command(Binary, "rm", "-f", o.Name).Run() // no --replace: clear any stale first
	if out, err := exec.Command(Binary, runArgs(o)...).CombinedOutput(); err != nil {
		return fmt.Errorf("container run %s: %s: %w", o.Name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Exec runs a command inside a pod and returns its combined output.
func (e Engine) Exec(name string, args ...string) ([]byte, error) {
	return e.ExecContext(context.Background(), name, args...)
}

// ExecContext is Exec bounded by ctx.
func (Engine) ExecContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	full := append([]string{"exec", name}, args...)
	out, err := exec.CommandContext(ctx, Binary, full...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("container exec %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return out, nil
}

// AttachCmd returns the interactive `container exec -it` unrun. It forwards
// TERM/COLORTERM, which exec drops: an empty TERM renders tmux scrambled.
func (Engine) AttachCmd(name string, args ...string) *exec.Cmd {
	full := append([]string{"exec", "-it"}, container.TermEnvArgs()...)
	full = append(full, name)
	full = append(full, args...)
	return exec.Command(Binary, full...)
}

// ExecInteractive runs a command wired to the caller's TTY — the human dial-in.
func (e Engine) ExecInteractive(name string, args ...string) error {
	c := e.AttachCmd(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Running reports whether the pod's micro-VM is running.
func (e Engine) Running(name string) bool { return e.RunningContext(context.Background(), name) }

// RunningContext is Running bounded by ctx, reading `.status.state` from inspect.
func (Engine) RunningContext(ctx context.Context, name string) bool {
	out, err := exec.CommandContext(ctx, Binary, "inspect", name).Output()
	if err != nil {
		return false // no such container / apiserver down — a legitimate "not running"
	}
	entries, err := parseInspect("inspect "+name, out)
	if err != nil || len(entries) == 0 {
		return false
	}
	return entries[0].Status.State == "running"
}

// Diagnose reports the `inspect` exit/stderr, entry count and state, so a "not running"
// verdict is explainable. Mirrors RunningContext's command to reflect the real probe.
func (Engine) Diagnose(ctx context.Context, name string) string {
	out, err := exec.CommandContext(ctx, Binary, "inspect", name).Output()
	msg := fmt.Sprintf("`%s inspect %s`: exit=%v, stdout=%dB", Binary, name, err, len(out))
	if ee, ok := err.(*exec.ExitError); ok {
		msg += fmt.Sprintf(", stderr=%q", strings.TrimSpace(string(ee.Stderr)))
	}
	var entries []inspectEntry
	if e := json.Unmarshal(out, &entries); e != nil {
		return msg + fmt.Sprintf(", json-error=%v", e)
	} else if len(entries) == 0 {
		return msg + ", entries=0"
	}
	return msg + fmt.Sprintf(", state=%q -> running=%v", entries[0].Status.State, entries[0].Status.State == "running")
}

// statsEntry is the slice of `container stats --format json` we read.
type statsEntry struct {
	ID               string `json:"id"`
	MemoryUsageBytes int64  `json:"memoryUsageBytes"`
	MemoryLimitBytes int64  `json:"memoryLimitBytes"`
}

// Stats snapshots memory via `container stats --no-stream` (one ~2s sample), which
// otherwise streams forever; the caller bounds it with ctx.
func (Engine) Stats(ctx context.Context, name string) (container.Usage, error) {
	out, err := exec.CommandContext(ctx, Binary, "stats", "--no-stream", "--format", "json", name).Output()
	if err != nil {
		if ctx.Err() != nil {
			return container.Usage{}, fmt.Errorf("container stats %s timed out: %w", name, ctx.Err())
		}
		return container.Usage{}, fmt.Errorf("container stats %s: %w", name, err)
	}
	var entries []statsEntry
	if e := json.Unmarshal(out, &entries); e != nil {
		return container.Usage{}, fmt.Errorf("container stats %s: parse JSON: %w", name, e)
	}
	if len(entries) == 0 {
		return container.Usage{}, fmt.Errorf("container stats %s: no sample returned", name)
	}
	return container.Usage{MemoryUsageBytes: entries[0].MemoryUsageBytes, MemoryLimitBytes: entries[0].MemoryLimitBytes}, nil
}

// MemoryCapacity reports what the fleet RESERVES against the memory it reserves from. A micro-VM
// takes its whole limit from the Mac when it starts, used or not, so the reservation total is what
// decides whether another agent starts at all — a fleet barely using its memory still cannot fit an
// agent whose reservation does not. Host memory is read here, in the backend, because which host it
// is (the Mac itself, for micro-VMs) is exactly what differs between backends.
func (Engine) MemoryCapacity(ctx context.Context) (container.Capacity, error) {
	out, err := exec.CommandContext(ctx, Binary, "ls", "--format", "json").Output()
	if err != nil {
		return container.Capacity{}, fmt.Errorf("container ls: %w", err)
	}
	entries, err := parseInspect("ls --format json", out)
	if err != nil {
		return container.Capacity{}, fmt.Errorf("container ls json: %w", err)
	}
	total, err := hostMemory(ctx)
	if err != nil {
		return container.Capacity{}, err
	}
	return container.Capacity{UsedBytes: reservedBytes(entries), TotalBytes: total, Basis: container.BasisReserved}, nil
}

// reservedBytes sums what the listed pods hold. `container ls` lists the running ones, which are
// the ones holding a reservation — a stopped micro-VM's limit is a number in its config, not memory
// anybody else is denied.
func reservedBytes(entries []inspectEntry) int64 {
	var n int64
	for _, e := range entries {
		n += e.Configuration.Resources.MemoryInBytes
	}
	return n
}

// hostMemory is the Mac's physical memory, read from sysctl — the pool micro-VMs reserve out of.
func hostMemory(ctx context.Context) (int64, error) {
	out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, fmt.Errorf("sysctl hw.memsize: %w", err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("sysctl hw.memsize: parse %q: %w", strings.TrimSpace(string(out)), err)
	}
	return n, nil
}

// Logs returns the last `tail` lines; `container logs` has no --tail, so trim here.
func (Engine) Logs(name string, tail int) string {
	out, err := exec.Command(Binary, "logs", name).CombinedOutput()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if tail > 0 && len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return strings.Join(lines, "\n")
}

// Info returns a short summary of a pod (name/state/image/id).
func (Engine) Info(name string) string {
	out, err := exec.Command(Binary, "inspect", name).Output()
	if err != nil {
		return ""
	}
	entries, err := parseInspect("inspect "+name, out)
	if err != nil || len(entries) == 0 {
		return ""
	}
	c := entries[0]
	var b strings.Builder
	fmt.Fprintf(&b, "state:    %s\n", c.Status.State)
	fmt.Fprintf(&b, "image:    %s\n", c.Configuration.Image.Reference)
	if c.Configuration.Resources.CPUs > 0 {
		fmt.Fprintf(&b, "cpus:     %d\n", c.Configuration.Resources.CPUs)
	}
	if c.Configuration.Resources.MemoryInBytes > 0 {
		fmt.Fprintf(&b, "memory:   %d MiB (limit)\n", c.Configuration.Resources.MemoryInBytes/(1024*1024))
	}
	if c.Configuration.Platform.OS != "" {
		fmt.Fprintf(&b, "platform: %s/%s\n", c.Configuration.Platform.OS, c.Configuration.Platform.Architecture)
	}
	if pid := hostPID(name); pid != "" {
		fmt.Fprintf(&b, "host pid: %s (micro-VM runtime process)\n", pid)
	}
	fmt.Fprintf(&b, "id:       %s", c.ID)
	return b.String()
}

// Rm force-removes a container (and its micro-VM).
func (e Engine) Rm(name string) error { return e.RmContext(context.Background(), name) }

// RmContext is Rm bounded by ctx: on cancellation the CLI is killed, and the error names the bound
// rather than the removal — stopping a micro-VM is part of the verb, so it is a slow one.
func (Engine) RmContext(ctx context.Context, name string) error {
	out, err := exec.CommandContext(ctx, Binary, "rm", "-f", name).CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("container rm %s: %w", name, ctx.Err())
	}
	return fmt.Errorf("container rm %s: %s: %w", name, strings.TrimSpace(string(out)), err)
}

// ListByLabelContext lists containers carrying label=value (empty value: any value, so
// one call covers every project). `container ls` has no `--filter`; we match client-side.
func (Engine) ListByLabelContext(ctx context.Context, label, value string) ([]string, error) {
	out, err := exec.CommandContext(ctx, Binary, "ls", "--all", "--format", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("container ls: %w", err)
	}
	entries, err := parseInspect("ls --format json", out)
	if err != nil {
		return nil, fmt.Errorf("container ls json: %w", err)
	}
	var names []string
	for _, e := range entries {
		got, ok := e.Configuration.Labels[label]
		if ok && (value == "" || got == value) {
			names = append(names, e.Configuration.ID)
		}
	}
	return names, nil
}

// Check verifies the `container` tool is installed and its service is running.
func (Engine) Check(w io.Writer) error {
	if _, err := exec.LookPath(Binary); err != nil {
		return fmt.Errorf("Apple `container` not found on PATH — install it (macOS 26) to run agents on this backend")
	}
	if ok, hint := (Engine{}).Healthy(); !ok {
		return fmt.Errorf("%s", hint)
	}
	return nil
}

// Healthy probes with `container ls`, which fails fast when the service is down.
func (Engine) Healthy() (ok bool, hint string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, Binary, "ls").Run(); err == nil {
		return true, ""
	}
	return false, "Apple `container` service isn't running — start it with `container system start`, then verify with `container ls`."
}

// EnsureImage builds the agent image via `container build` when the recipe is stale.
func (Engine) EnsureImage(root, containerfile string, out io.Writer) (string, error) {
	return container.EnsureImageWith(root, containerfile, out, appleBuilder{})
}

// RebuildImage forces a rebuild (re-pulling the base) — for picking up a newer base.
func (Engine) RebuildImage(root, containerfile string, out io.Writer) (string, error) {
	return container.RebuildImageWith(root, containerfile, out, appleBuilder{})
}

// appleBuilder is the Apple-`container` slice of image building.
type appleBuilder struct{}

func (appleBuilder) ImageExists(ref string) (bool, error) {
	// NB: the subcommand is `image` (singular); `images` exits non-zero, which once read
	// as "absent" and rebuilt on every launch.
	out, err := exec.Command(Binary, "image", "inspect", ref).CombinedOutput()
	if err == nil {
		return true, nil
	}
	// "image not found" + exit 1 is a legitimate absent; anything else (service down,
	// bad args) is a real error, surfaced rather than masqueraded as "absent".
	if strings.Contains(string(out), "not found") {
		return false, nil
	}
	return false, fmt.Errorf("container image inspect %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
}

func (appleBuilder) Build(ref, ctxDir, dockerfile string, pull bool, out io.Writer) error {
	// pull is best-effort: `container build` has no re-pull flag, so a forced rebuild
	// may reuse the local base. (podman does a real --pull=always.)
	_ = pull
	cmd := exec.Command(Binary, "build", "-t", ref, "-f", dockerfile, ctxDir)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("container build failed: %w", err)
	}
	return nil
}

// sortedKeys returns map keys in sorted order for deterministic argv.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
