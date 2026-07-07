// package: adapter/applecontainer / applecontainer
// type:    adapter (external tool: Apple `container`) — implements container.Runtime
// job:     the Apple-`container` backend for the container-runtime port (macOS 26):
//          each agent pod is its OWN micro-VM, so one agent's crash/OOM can't take
//          down the others. Maps run/exec/attach/liveness/logs/remove/orphan-list/
//          image-build onto the `container` CLI.
// limits:  implements container.Runtime; wired in at the composition root. macOS
//          only (needs the `container` service + a Linux kernel per micro-VM).
package applecontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/container"
)

// Binary is the Apple container executable.
var Binary = "container"

// Engine is the Apple-`container` implementation of container.Runtime.
type Engine struct{}

// inspectEntry is the slice of `container inspect`/`ls --format json` we read.
// NOTE: configuration.image is an OBJECT ({reference, descriptor}), not a string —
// modelling it as a string made json.Unmarshal fail on the whole entry, and because
// the callers swallowed that error as "false", a live container read as down. Keep
// this shape faithful to `container inspect`'s real output.
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
		Labels map[string]string `json:"labels"`
	} `json:"configuration"`
}

// parseInspect unmarshals `container inspect`/`ls` JSON. A shape mismatch here is a
// BUG in our model of the tool's output — never a normal "not present" — so it is
// logged loudly instead of being swallowed into a misleading false/empty. (A silent
// return-false on exactly this kind of error once cost hours of misdiagnosis.)
func parseInspect(what string, raw []byte) ([]inspectEntry, error) {
	var entries []inspectEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		log.Printf("applecontainer: %s returned JSON we can't parse (adapter bug — schema drift?): %v", what, err)
		return nil, err
	}
	return entries, nil
}

// runArgs builds `container run -d …`. Unlike podman there is no `--userns` (each
// micro-VM maps mounts itself) and no `--replace` (Run removes any stale first).
func runArgs(o container.RunOpts) []string {
	args := []string{"run", "-d", "--name", o.Name}
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

// AttachCmd returns (without running) the interactive `container exec -it` command.
func (Engine) AttachCmd(name string, args ...string) *exec.Cmd {
	full := append([]string{"exec", "-it", name}, args...)
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

// Diagnose reports exactly what the running probe sees: the `inspect` exit/stderr,
// how many entries parsed, and the state string — so a "not running" verdict is
// explainable (tool missing, apiserver error, unexpected state) rather than a
// silent false. It mirrors RunningContext's command so it reflects the real probe.
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

// Logs returns the last `tail` lines of a container's output. Apple `container logs`
// has no --tail, so we trim client-side. Best-effort.
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
	return fmt.Sprintf("name:  %s\nstate: %s\nimage: %s\nid:    %s", name, c.Status.State, c.Configuration.Image.Reference, c.ID)
}

// Rm force-removes a container (and its micro-VM).
func (Engine) Rm(name string) error {
	if out, err := exec.Command(Binary, "rm", "-f", name).CombinedOutput(); err != nil {
		return fmt.Errorf("container rm %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListByLabelContext lists containers carrying label=value. Apple `container ls` has
// no `--filter`, so we list all as JSON and match the label client-side.
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
		if e.Configuration.Labels[label] == value {
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

// Healthy is a fast, time-bounded probe: `container ls` fails quickly when the
// service isn't started.
func (Engine) Healthy() (ok bool, hint string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, Binary, "ls").Run(); err == nil {
		return true, ""
	}
	return false, "Apple `container` service isn't running — start it with `container system start`, then verify with `container ls`."
}

// EnsureImage builds the agent image via `container build` when the recipe is stale.
func (Engine) EnsureImage(root string, out io.Writer) error {
	return container.EnsureImageWith(root, out, appleBuilder{})
}

// appleBuilder is the Apple-`container` slice of image building.
type appleBuilder struct{}

func (appleBuilder) ImageExists() (bool, error) {
	// NB: the subcommand is `image` (singular); `images` is not a valid subcommand and
	// exits non-zero, which — when this returned a bare bool — silently read as "absent"
	// and rebuilt on every launch.
	out, err := exec.Command(Binary, "image", "inspect", container.ImageName).CombinedOutput()
	if err == nil {
		return true, nil
	}
	// A genuinely-missing image is a legitimate "absent", not a failure: `container
	// image inspect` prints "image not found" and exits 1. Anything else (service
	// down, bad args) is a real error we surface instead of masquerading as "absent".
	if strings.Contains(string(out), "not found") {
		return false, nil
	}
	return false, fmt.Errorf("container image inspect %s: %s: %w", container.ImageName, strings.TrimSpace(string(out)), err)
}

func (appleBuilder) Build(ctxDir, dockerfile string, out io.Writer) error {
	cmd := exec.Command(Binary, "build", "-t", container.ImageName, "-f", dockerfile, ctxDir)
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
