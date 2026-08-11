// package: hub/agent / memory
// type:    logic (per-agent memory config)
// job:     resolve, validate, and set an agent pod's RAM limit — the per-agent value
// (store.Agent.Memory) with a modest default when unset. Applied at Launch
// (RunOpts.Memory); a change takes effect on the agent's next start.
// limits:  config only; the limit is enforced by the container runtime, not here.
package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/flo-at/sindri/internal/container"
)

// fallbackMemory is used only when no runtime is wired (a worker-only process, a test): the hub
// must still print a number rather than an empty cell.
const fallbackMemory = "2g"

// MemoryOrDefault resolves an agent's configured memory limit, falling back to the RUNTIME's
// default when unset. The right default is the backend's to state, not the hub's: a shared-kernel
// container's limit is a ceiling it grows into, a micro-VM's is a reservation taken from the host
// whether used or not, so the same number does not suit both. Per-agent config still wins
// (store.Agent.Memory, via `agent new --memory` / `agent memory` / the TUI).
func MemoryOrDefault(m string) string {
	if strings.TrimSpace(m) != "" {
		return strings.TrimSpace(m)
	}
	if d := container.DefaultMemory(); d != "" {
		return d
	}
	return fallbackMemory
}

// memoryRe validates a memory limit like "2g", "512m", "2048", "1gb".
var memoryRe = regexp.MustCompile(`(?i)^[0-9]+(k|m|g)?b?$`)

// ValidMemory reports whether m is empty (use default) or a well-formed size — used
// by the hub to validate a memory value at agent creation, before this service.
func ValidMemory(m string) bool {
	m = strings.TrimSpace(m)
	return m == "" || memoryRe.MatchString(m)
}

// SetMemory updates an agent's RAM limit (e.g. "4g"); "" resets it to the hub
// default. It takes effect on the agent's next start/restart — a running pod's limit
// is fixed when the pod is created.
func (s *Service) SetMemory(project, name, memory string) error {
	if !ValidMemory(memory) {
		return fmt.Errorf("invalid memory %q (e.g. 2g, 512m)", memory)
	}
	ps := s.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	a.Memory = strings.TrimSpace(memory)
	if err := ps.PutAgent(a); err != nil {
		return err
	}
	defer s.deps.Notify()
	return ps.Log(name, "config", "memory="+MemoryOrDefault(a.Memory))
}
