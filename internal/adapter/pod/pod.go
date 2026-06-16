// package: adapter/pod / pod
// type:    adapter (external tool: podman)
// job:     wrap podman — run a detached agent pod, exec into it (the hub's reach
//          into a pod, e.g. to drive tmux), check existence, remove, and list
//          sindri pods. The only place podman is invoked.
// limits:  knows nothing of agents, roles, tmux semantics, or the hub; callers
//          compose it (e.g. pod.Exec(c, tmux.SendText(...)...)).
package pod

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Binary is the podman executable; overridable for tests/alternate runtimes.
var Binary = "podman"

// Mount is one bind mount.
type Mount struct {
	Host      string
	Container string
	Mode      string // "ro" | "rw"; ",z" relabel is appended automatically
}

// RunOpts configures a detached agent pod.
type RunOpts struct {
	Name       string            // container name
	Image      string            // image ref
	Labels     map[string]string // e.g. sindri.project, sindri.agent
	Env        map[string]string
	Mounts     []Mount
	Workdir    string
	Entrypoint []string // command + args run as the container's process
}

// UserNS maps the host user to the container's sindri uid/gid (1000) so
// host-owned mounts (workspace, the agent socket) appear owned by the in-pod
// user regardless of the host uid — plain keep-id breaks when the host uid is
// not 1000 (e.g. a subuid-mapped rootless setup).
const UserNS = "keep-id:uid=1000,gid=1000"

// RunArgs builds the `podman run -d ...` argv (pure, for testing). Mounts and
// env are emitted in a stable order so the output is deterministic.
func RunArgs(o RunOpts) []string {
	args := []string{"run", "-d", "--name", o.Name, "--userns=" + UserNS}
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
	args = append(args, o.Image)
	args = append(args, o.Entrypoint...)
	return args
}

// Run launches a detached pod.
func Run(o RunOpts) error {
	if out, err := exec.Command(Binary, RunArgs(o)...).CombinedOutput(); err != nil {
		return fmt.Errorf("podman run %s: %s: %w", o.Name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Exec runs a command inside a pod and returns its combined output.
func Exec(name string, args ...string) ([]byte, error) {
	full := append([]string{"exec", name}, args...)
	out, err := exec.Command(Binary, full...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("podman exec %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return out, nil
}

// ExecInteractive runs a command inside a pod wired to the caller's TTY — used
// for `attach`, where the human shares the agent's terminal.
func ExecInteractive(name string, args ...string) error {
	full := append([]string{"exec", "-it", name}, args...)
	c := exec.Command(Binary, full...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Running reports whether a container exists and is running.
func Running(name string) bool {
	out, err := exec.Command(Binary, "inspect", "-f", "{{.State.Running}}", name).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// Rm force-removes a container.
func Rm(name string) error {
	if out, err := exec.Command(Binary, "rm", "-f", name).CombinedOutput(); err != nil {
		return fmt.Errorf("podman rm %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// sortedKeys returns map keys in sorted order for deterministic argv.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// simple insertion sort — maps here are tiny
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
