// package: adapter/pod / pod
// type:    adapter (external tool: podman) — implements container.Runtime
// job:     the podman backend for the container-runtime port: run a detached agent
//          pod, exec/attach, liveness, logs, remove, orphan-list, VM pre-flight, and
//          build the agent image via `podman build`. The only place podman is run.
// limits:  knows nothing of agents, roles, or the hub; it implements the
//          container.Runtime interface and is wired in at the composition root.
package pod

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/container"
)

// Binary is the podman executable; overridable for tests.
var Binary = "podman"

// Engine is the podman implementation of container.Runtime.
type Engine struct{}

// Name identifies this backend for humans.
func (Engine) Name() string { return "podman" }

// AgentChannel: podman resolves the magic name host.containers.internal to a
// forwarded route to host services, so pods dial that name and the hub binds
// loopback.
func (Engine) AgentChannel() (container.NetChannel, error) {
	return container.NetChannel{BindAddr: "127.0.0.1", DialHost: "host.containers.internal"}, nil
}

// UserNS maps the host user to the container's sindri uid/gid (1000) so
// host-owned mounts appear owned by the in-pod user regardless of the host uid.
const UserNS = "keep-id:uid=1000,gid=1000"

// RunArgs builds the `podman run -d ...` argv (pure, for testing). Mounts and env
// are emitted in a stable order so the output is deterministic.
func RunArgs(o container.RunOpts) []string {
	// --replace tears down any stale container of the same name first.
	args := []string{"run", "-d", "--replace", "--name", o.Name, "--userns=" + UserNS}
	if o.Memory != "" {
		args = append(args, "-m", o.Memory)
	}
	for _, k := range sortedKeys(o.Labels) {
		args = append(args, "--label", k+"="+o.Labels[k])
	}
	for _, k := range sortedKeys(o.Env) {
		args = append(args, "-e", k+"="+o.Env[k])
	}
	for _, m := range o.Mounts {
		mode := m.Mode
		if mode == "" {
			mode = "rw"
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s,z", m.Host, m.Container, mode))
	}
	if o.Workdir != "" {
		args = append(args, "-w", o.Workdir)
	}
	for _, d := range o.Devices {
		args = append(args, "--device", d)
	}
	for _, s := range o.SecurityOpt {
		args = append(args, "--security-opt", s)
	}
	args = append(args, o.Image)
	args = append(args, o.Entrypoint...)
	return args
}

// Run launches a detached pod.
func (Engine) Run(o container.RunOpts) error {
	if out, err := exec.Command(Binary, RunArgs(o)...).CombinedOutput(); err != nil {
		return fmt.Errorf("podman run %s: %s: %w", o.Name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Exec runs a command inside a pod and returns its combined output.
func (e Engine) Exec(name string, args ...string) ([]byte, error) {
	return e.ExecContext(context.Background(), name, args...)
}

// ExecContext is Exec bounded by ctx: when ctx is cancelled the podman process is
// killed and the call returns promptly, so a wedged container can't stall the caller.
func (Engine) ExecContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	full := append([]string{"exec", name}, args...)
	out, err := exec.CommandContext(ctx, Binary, full...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("podman exec %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return out, nil
}

// AttachCmd returns (without running) the interactive `podman exec -it` command.
func (Engine) AttachCmd(name string, args ...string) *exec.Cmd {
	full := append([]string{"exec", "-it", name}, args...)
	return exec.Command(Binary, full...)
}

// ExecInteractive runs a command inside a pod wired to the caller's TTY — used for
// `attach`, where the human shares the agent's terminal.
func (e Engine) ExecInteractive(name string, args ...string) error {
	c := e.AttachCmd(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Check verifies podman is installed and its service/VM is reachable, auto-starting
// a stopped VM on macOS/Windows (but never creating one), returning an actionable
// error otherwise.
func (Engine) Check(w io.Writer) error {
	if _, err := exec.LookPath(Binary); err != nil {
		return fmt.Errorf("%s not found on PATH — install podman (https://podman.io) to run agents", Binary)
	}
	detail, ok := reachable()
	if ok {
		return nil
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		switch machineState() {
		case machineStopped:
			fmt.Fprintln(w, "podman VM is stopped — starting it (this can take a minute)…")
			if out, err := exec.Command(Binary, "machine", "start").CombinedOutput(); err != nil {
				return fmt.Errorf("podman VM is stopped and `podman machine start` failed: %s", lastLine(string(out)))
			}
			if _, ok := reachable(); ok {
				fmt.Fprintln(w, "podman VM is up.")
				return nil
			}
		case machineMissing:
			return fmt.Errorf("no podman VM exists yet — on %s podman runs in a VM.\n"+
				"Create and start it once with:\n"+
				"    podman machine init\n"+
				"    podman machine start\n"+
				"then re-run. (sindri auto-starts the VM after that, but won't create it for "+
				"you — `podman machine init` downloads a VM image and provisions ~100GiB of disk.)", runtime.GOOS)
		}
	}
	if detail == "" {
		detail = "podman info failed"
	}
	return fmt.Errorf("podman is installed but not reachable: %s\n"+
		"On macOS/Windows podman runs in a VM — run `podman machine init` (first time) then "+
		"`podman machine start`, and verify with `podman info`", detail)
}

// Healthy is a fast, time-bounded reachability probe: `podman info` with a short
// timeout, so a wedged VM fails quickly instead of hanging the command.
func (Engine) Healthy() (ok bool, hint string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, Binary, "info").Run(); err == nil {
		return true, ""
	}
	return false, "podman isn't reachable — agents can't run until it is. On macOS/Windows: `podman machine start` (or stop then start if it's wedged), then verify with `podman info`."
}

// Running reports whether a container exists and is running.
func (e Engine) Running(name string) bool { return e.RunningContext(context.Background(), name) }

// RunningContext is Running bounded by ctx: on cancellation the podman process is
// killed and it reports false, so a stalled inspect degrades to "down".
func (Engine) RunningContext(ctx context.Context, name string) bool {
	out, err := exec.CommandContext(ctx, Binary, "inspect", "-f", "{{.State.Running}}", name).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// Diagnose reports exactly what the running probe sees: the `inspect` exit/stderr
// and the raw {{.State.Running}} value — so a "not running" verdict is explainable
// (no such container, VM unreachable, unexpected value) rather than a silent false.
func (Engine) Diagnose(ctx context.Context, name string) string {
	out, err := exec.CommandContext(ctx, Binary, "inspect", "-f", "{{.State.Running}}", name).Output()
	msg := fmt.Sprintf("`%s inspect %s`: exit=%v, out=%q", Binary, name, err, strings.TrimSpace(string(out)))
	if ee, ok := err.(*exec.ExitError); ok {
		msg += fmt.Sprintf(", stderr=%q", strings.TrimSpace(string(ee.Stderr)))
	}
	return msg
}

// Stats returns a memory snapshot for a running container. Podman renders memory as
// a human string "usage / limit" (e.g. "543.9MB / 1.074GB"); parse both sides.
// --no-stream takes a single sample; the caller bounds it with ctx.
func (Engine) Stats(ctx context.Context, name string) (container.Usage, error) {
	out, err := exec.CommandContext(ctx, Binary, "stats", "--no-stream", "--format", "{{.MemUsage}}", name).Output()
	if err != nil {
		if ctx.Err() != nil {
			return container.Usage{}, fmt.Errorf("podman stats %s timed out: %w", name, ctx.Err())
		}
		return container.Usage{}, fmt.Errorf("podman stats %s: %w", name, err)
	}
	usage, limit, ok := strings.Cut(strings.TrimSpace(string(out)), "/")
	if !ok {
		return container.Usage{}, fmt.Errorf("podman stats %s: unexpected mem usage %q", name, strings.TrimSpace(string(out)))
	}
	u, uerr := parseByteSize(usage)
	l, lerr := parseByteSize(limit)
	if uerr != nil || lerr != nil {
		return container.Usage{}, fmt.Errorf("podman stats %s: parse %q: %v / %v", name, strings.TrimSpace(string(out)), uerr, lerr)
	}
	return container.Usage{MemoryUsageBytes: u, MemoryLimitBytes: l}, nil
}

// parseByteSize parses a human byte size as podman prints it ("543.9MB", "1.074GB",
// decimal/1000-based) into bytes.
func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	mult := 1.0
	for _, u := range []struct {
		suffix string
		m      float64
	}{{"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1}} {
		if strings.HasSuffix(s, u.suffix) {
			mult, s = u.m, strings.TrimSuffix(s, u.suffix)
			break
		}
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", s, err)
	}
	return int64(f * mult), nil
}

// Logs returns the last `tail` lines of a container's stdout/stderr. Best-effort.
func (Engine) Logs(name string, tail int) string {
	out, err := exec.Command(Binary, "logs", "--tail", strconv.Itoa(tail), name).CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

// Info returns a labelled summary of a container via `podman ps -a`, or "" if none.
func (Engine) Info(name string) string {
	const f = "name:    {{.Names}}\nstate:   {{.State}}\nstatus:  {{.Status}}\nimage:   {{.Image}}\ncreated: {{.CreatedAt}}\nports:   {{.Ports}}\nid:      {{.ID}}"
	out, err := exec.Command(Binary, "ps", "-a", "--filter", "name=^"+name+"$", "--format", f).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Rm force-removes a container.
func (Engine) Rm(name string) error {
	if out, err := exec.Command(Binary, "rm", "-f", name).CombinedOutput(); err != nil {
		return fmt.Errorf("podman rm %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListByLabelContext returns the names of containers carrying label=value — used to
// find sindri pods for orphan detection. Bounded by ctx.
func (Engine) ListByLabelContext(ctx context.Context, label, value string) ([]string, error) {
	out, err := exec.CommandContext(ctx, Binary, "ps", "-a",
		"--filter", "label="+label+"="+value, "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, fmt.Errorf("podman ps: %w", err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// EnsureImage builds the agent image via `podman build` when the recipe is stale,
// returning the image reference to run.
func (Engine) EnsureImage(root, containerfile string, out io.Writer) (string, error) {
	return container.EnsureImageWith(root, containerfile, out, podmanBuilder{})
}

// RebuildImage forces a rebuild (re-pulling the base) — for picking up a newer base.
func (Engine) RebuildImage(root, containerfile string, out io.Writer) (string, error) {
	return container.RebuildImageWith(root, containerfile, out, podmanBuilder{})
}

// podmanBuilder is the podman slice of image building for container.EnsureImageWith.
type podmanBuilder struct{}

func (podmanBuilder) ImageExists(ref string) (bool, error) {
	cmd := exec.Command(Binary, "image", "exists", ref)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// `podman image exists` documents exit 1 as "image does not exist" — a legitimate
	// absent, with nothing on stderr. Any other exit (e.g. 125, VM unreachable) is a
	// real error we surface instead of collapsing to "absent, rebuild".
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 && stderr.Len() == 0 {
		return false, nil
	}
	return false, fmt.Errorf("podman image exists %s: %s: %w", ref, strings.TrimSpace(stderr.String()), err)
}

func (podmanBuilder) Build(ref, ctxDir, dockerfile string, pull bool, out io.Writer) error {
	// Capture podman's output alongside streaming it, so a failure carries the actual
	// diagnostic — not a bare "exit status 125".
	var captured bytes.Buffer
	args := []string{"build", "-t", ref, "-f", dockerfile}
	if pull {
		// Re-pull the base (FROM …:latest) so a rebuild actually picks up a newer base
		// image; without this podman reuses the locally-cached base and Go never moves.
		args = append(args, "--pull=always")
	}
	args = append(args, ctxDir)
	cmd := exec.Command(Binary, args...)
	cmd.Stdout = io.MultiWriter(out, &captured)
	cmd.Stderr = io.MultiWriter(out, &captured)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("podman build failed (%v):\n%s", err, buildFailureDetail(captured.String()))
	}
	return nil
}

// reachable reports whether `podman info` succeeds, returning the trimmed last line
// of its output (the reason) when it doesn't.
func reachable() (detail string, ok bool) {
	out, err := exec.Command(Binary, "info").CombinedOutput()
	if err == nil {
		return "", true
	}
	return lastLine(string(out)), false
}

type machineStatus int

const (
	machineUnknown machineStatus = iota
	machineMissing
	machineStopped
	machineRunning
)

// machineState reports the podman VM state via `podman machine list`.
func machineState() machineStatus {
	out, err := exec.Command(Binary, "machine", "list", "--format", "json").Output()
	if err != nil {
		return machineUnknown
	}
	var machines []struct {
		Running bool `json:"Running"`
	}
	if err := json.Unmarshal(out, &machines); err != nil {
		return machineUnknown
	}
	if len(machines) == 0 {
		return machineMissing
	}
	for _, m := range machines {
		if m.Running {
			return machineRunning
		}
	}
	return machineStopped
}

// lastLine returns the trimmed final non-empty line of s.
func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[i+1:])
	}
	return s
}

// buildFailureDetail distills podman's build output: the meaningful tail plus a hint
// for the most common cause (podman not reachable, notably a stopped macOS VM).
func buildFailureDetail(out string) string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, "\r"))
		}
	}
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	detail := strings.Join(lines, "\n")
	low := strings.ToLower(out)
	unreachable := detail == "" ||
		strings.Contains(low, "cannot connect") ||
		strings.Contains(low, "connection refused") ||
		strings.Contains(low, "no such host") ||
		strings.Contains(low, "machine") ||
		strings.Contains(low, "is the podman")
	if unreachable {
		hint := "hint: podman doesn't look reachable. On macOS/Windows it runs in a VM — run `podman machine init` (first time) then `podman machine start`; check with `podman info`."
		if detail == "" {
			return hint
		}
		detail += "\n" + hint
	}
	return detail
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
