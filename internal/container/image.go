// package: container / image
// type:    adapter (podman)
// job:     the agent image identity (ImageName) and build — rebuilds via podman
//          when anything under container/ changes or the weekly cache key is
//          stale.
// limits:  worker/reviewer container lifecycle lives in internal/worker.
package container

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const ImageName = "sindri-agent:test"

// Ensure builds the container image if anything under container/ changed or the
// weekly cache key is stale. Build progress is written to out (so the hub can
// tee it into an agent's live-screen region during launch).
func Ensure(projectRoot string, out io.Writer) error {
	dir := projectRoot + "/container"
	dockerfile := dir + "/Dockerfile"
	if _, err := os.Stat(dockerfile); err != nil {
		if exec.Command("podman", "image", "exists", ImageName).Run() == nil {
			return nil
		}
		return fmt.Errorf("no Dockerfile and no %s image", ImageName)
	}

	year, week := time.Now().ISOWeek()
	h := sha256.New()
	// Hash every file under container/ (Dockerfile, sindri-agent.sh, …) so a
	// change to the entrypoint — not just the Dockerfile — triggers a rebuild.
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if data, e := os.ReadFile(p); e == nil {
			h.Write([]byte(p))
			h.Write(data)
		}
		return nil
	})
	h.Write([]byte(fmt.Sprintf("%d-%d", year, week)))
	buildKey := fmt.Sprintf("%x", h.Sum(nil))[:16]

	cacheFile := projectRoot + "/.worktrees/.build-key"
	if cached, err := os.ReadFile(cacheFile); err == nil && strings.TrimSpace(string(cached)) == buildKey {
		return nil
	}

	fmt.Fprintf(out, "Building container image...\n")
	_ = os.MkdirAll(projectRoot+"/bin", 0755)
	for _, bin := range []string{"td", "yq"} {
		if path, err := exec.LookPath(bin); err == nil {
			data, _ := os.ReadFile(path)
			_ = os.WriteFile(projectRoot+"/bin/"+bin, data, 0755)
		}
	}

	cmd := exec.Command("podman", "build", "-t", ImageName, "-f", dockerfile, projectRoot)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("image build failed: %w", err)
	}

	_ = os.MkdirAll(projectRoot+"/.worktrees", 0755)
	_ = os.WriteFile(cacheFile, []byte(buildKey), 0644)
	return nil
}
