// package: hub/agent / memory
// type:    logic (per-agent memory config)
// job:     resolve, validate, and set an agent pod's RAM limit — the per-agent value
//          (store.Agent.Memory) with a modest default when unset. Applied at Launch
//          (RunOpts.Memory); a change takes effect on the agent's next start.
// limits:  config only; the limit is enforced by the container runtime, not here.
package agent

import (
	"fmt"
	"regexp"
	"strings"
)

// defaultAgentMemory caps an agent pod's RAM when it has none configured. The
// runtime default (1GiB) is too little for a worker running the Go compiler,
// linters, and Claude Code at once; but each apple micro-VM takes its RAM from the
// host, so this stays modest — on a small Mac a few agents at once shouldn't crowd
// it out. It's per-agent configurable (store.Agent.Memory) via `agent new --memory`
// / `agent memory` / the TUI; this is only the fallback.
const defaultAgentMemory = "2g"

// MemoryOrDefault resolves an agent's configured memory limit, falling back to the
// hub default when unset.
func MemoryOrDefault(m string) string {
	if strings.TrimSpace(m) != "" {
		return strings.TrimSpace(m)
	}
	return defaultAgentMemory
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
