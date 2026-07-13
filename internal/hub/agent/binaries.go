// package: hub/agent / binaries
// type:    logic
// job:     locate the sibling binaries an agent pod needs (sindri-worker, brokkr,
//          and the linux brokkr mounted into pods) — next to the running sindri
//          executable first, then on PATH.
// limits:  path resolution only; it doesn't run or mount anything (-> the hub's
//          launch path wires the mounts and entrypoint).
package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Binary locates the thin browser binary on the host: next to the running
// sindri binary first, then on PATH.
func Binary() (string, error) {
	const name = "sindri-worker"
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s binary not found — run 'make build/install'", name)
}

// BrokkrBinary locates the brokkr toolbelt binary (which runs the linters): next
// to the running sindri binary first, then on PATH. The lint gate shells out to
// it (brokkr ships alongside sindri).
func BrokkrBinary() (string, error) {
	const name = "brokkr"
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("brokkr binary not found — it ships with sindri ('make install')")
}

// BrokkrLinuxBinary locates a linux brokkr to mount into an agent pod (pods are
// always linux, whatever the host). It prefers the cross-built "brokkr-linux"
// shipped next to sindri / on PATH; on a linux host the plain host brokkr is
// itself linux, so we fall back to that. macOS with no brokkr-linux has neither,
// so the caller simply skips the mount.
func BrokkrLinuxBinary() (string, error) {
	const name = "brokkr-linux"
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if runtime.GOOS == "linux" {
		return BrokkrBinary()
	}
	return "", fmt.Errorf("%s binary not found — it ships with sindri ('make install')", name)
}
