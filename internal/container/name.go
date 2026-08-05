// package: container / name
// type:    logic (pure derivation)
// job:     an agent's container name — scoped by repo so two repos reusing an agent
// name don't collide in the runtime's host-global namespace. Front-ends need
// this to address a container directly (podman inspect/logs), so it lives
// here rather than behind the hub.
// limits:  pure string derivation only; no runtime calls.
package container

import (
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/api"
)

// AgentContainer is an agent's repo-scoped container name: sindri-<slug>-<digest>-<name>.
func AgentContainer(root, agent string) string {
	return "sindri-" + repoSlug(root) + "-" + api.RepoTag(root) + "-" + agent
}

// repoSlug is the directory name, lowercased and podman-safe, so `podman ps` is eyeballable.
func repoSlug(root string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(filepath.Base(root)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		s = "repo"
	}
	if len(s) > 16 {
		s = s[:16]
	}
	return s
}
