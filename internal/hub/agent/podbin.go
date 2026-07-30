// package: hub/agent / podbin
// type:    logic
// job:     keep pod-bin holding current copies of the host-built tools agents run, so a
// rebuild reaches running agents without recreating their containers.
// limits:  copies and reports; mounting is the launch path's, building the Makefile's.
package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/flo-at/sindri/internal/tools/paths"
)

// SyncPodBin refreshes pod-bin from the host's installed tools, returning the pod names it
// updated. Copy-then-rename, so a running agent sees the new file on its next exec and an
// in-flight process keeps its old inode; writing in place would hit ETXTBSY. A missing host
// binary is collected as an error, not fatal — a host with no cross-built brokkr has none to
// publish. Unchanged tools are skipped.
func SyncPodBin() (updated []string, err error) {
	dir := paths.PodBinDir()
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return nil, fmt.Errorf("pod-bin dir: %w", mkErr)
	}
	var problems []error
	for _, t := range paths.PodTools {
		src, rErr := hostToolPath(t.HostName)
		if rErr != nil {
			problems = append(problems, rErr)
			continue
		}
		dst := filepath.Join(dir, t.PodName)
		same, cErr := sameContent(src, dst)
		if cErr != nil {
			problems = append(problems, fmt.Errorf("%s: %w", t.PodName, cErr))
			continue
		}
		if same {
			continue
		}
		if pErr := publish(src, dst); pErr != nil {
			problems = append(problems, fmt.Errorf("%s: %w", t.PodName, pErr))
			continue
		}
		updated = append(updated, t.PodName)
	}
	return updated, errors.Join(problems...)
}

// hostToolPath defers to the resolver that already owns each lookup, so pod-bin is filled
// from exactly the binary the rest of the hub would have used.
func hostToolPath(hostName string) (string, error) {
	switch hostName {
	case "brokkr-linux":
		return BrokkrLinuxBinary()
	case "sindri-worker":
		return Binary()
	}
	return "", fmt.Errorf("no resolver for host tool %q", hostName)
}

// sameContent compares size and mtime. A heuristic on purpose: hashing two 15 MB binaries on
// every hub start would cost more than it saves, and a rebuild moves both.
func sameContent(src, dst string) (bool, error) {
	s, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	d, err := os.Stat(dst)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.Size() == d.Size() && s.ModTime().Equal(d.ModTime()), nil
}

// publish copies src to dst via a temp file in dst's directory and an atomic rename, keeping
// the source's mtime so sameContent recognises it next time.
func publish(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	// Same directory as dst, so the rename stays within one filesystem and is atomic.
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil { // it has to be runnable in the pod
		return err
	}
	if err := os.Chtimes(tmpName, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}
