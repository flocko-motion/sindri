// package: hub/agent / memory
// type:    logic (per-agent memory config)
// job:     resolve, validate and set an agent pod's RAM limit (store.Agent.Memory, with a
// modest default when unset), and count the machine's free memory in agents of
// that size. Applied at Launch; a change takes effect on the next start.
// limits:  config and arithmetic; the limit is enforced by the runtime, and what the
// machine has left is the runtime's to report (-> container.MemoryCapacity).
package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/flo-at/sindri/internal/api"
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

// Headroom folds one capacity reading into the board's figure: what is free, and how many more
// default-size agents fit there — the question being asked, which a percentage leaves the reader
// to divide out. A zero total stays unknown: an unmeasured machine has no free memory to report.
func Headroom(c container.Capacity) api.FleetMemory {
	m := api.FleetMemory{UsedBytes: c.UsedBytes, TotalBytes: c.TotalBytes, Basis: c.Basis}
	if !m.Known() {
		return api.FleetMemory{}
	}
	if m.AgentBytes = parseMemory(MemoryOrDefault("")); m.AgentBytes > 0 {
		m.Fits = int(m.FreeBytes() / m.AgentBytes)
	}
	return m
}

// parseMemory reads a limit in the form the runtimes take it ("2g", "512m", "1gb", plain bytes)
// into bytes, binary units as the runtimes apply them; 0 when it is not a size.
func parseMemory(m string) int64 {
	m = strings.ToLower(strings.TrimSpace(m))
	if !ValidMemory(m) || m == "" {
		return 0
	}
	m = strings.TrimSuffix(m, "b")
	unit := int64(1)
	for _, u := range []struct {
		suffix string
		size   int64
	}{{"k", 1 << 10}, {"m", 1 << 20}, {"g", 1 << 30}} {
		if strings.HasSuffix(m, u.suffix) {
			unit, m = u.size, strings.TrimSuffix(m, u.suffix)
			break
		}
	}
	n, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		return 0
	}
	return n * unit
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
