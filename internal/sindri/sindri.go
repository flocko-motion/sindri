// package: sindri / sindri
// type:    adapter (filesystem)
// job:     the `.sindri/` project scaffold and the durable agent index — the
//          source of truth for which agents exist (one JSON file per agent).
// limits:  identity only; agent live progress (task/status) lives in each
//          workspace's `.sindri-task`. Reconciliation against podman/git lives
//          in internal/worker.
package sindri

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Agent is one entry in the index: an agent's durable identity. Live progress
// (current task, status) is NOT stored here — it lives in the agent's workspace.
type Agent struct {
	Name      string `json:"name"`
	Role      string `json:"role"` // "worker" | "reviewer"
	Mode      string `json:"mode"` // worker: "next" | "choose"; reviewer: ""
	Base      string `json:"base"` // base branch (worker)
	Workspace string `json:"workspace"`
	CreatedAt string `json:"created_at"`
}

// Config is the project-level `.sindri/config.json`. Kept intentionally thin;
// knobs (dwarf pool, image tag, …) can be added later.
type Config struct {
	Version int `json:"version"`
}

const configVersion = 1

// Dir is the `.sindri/` directory for a project.
func Dir(projectRoot string) string { return filepath.Join(projectRoot, ".sindri") }

// AgentsDir is `.sindri/agents/`, the roster directory.
func AgentsDir(projectRoot string) string { return filepath.Join(Dir(projectRoot), "agents") }

// Exists reports whether the project has been initialised.
func Exists(projectRoot string) bool {
	_, err := os.Stat(Dir(projectRoot))
	return err == nil
}

// agentPath is the index file for a single agent.
func agentPath(projectRoot, name string) string {
	return filepath.Join(AgentsDir(projectRoot), name+".json")
}

// EnsureSindri creates the `.sindri/` scaffold if absent: the agents directory,
// a default config, the singleton reviewer entry, and a `.gitignore` entry.
// It is idempotent.
func EnsureSindri(projectRoot string) error {
	if err := os.MkdirAll(AgentsDir(projectRoot), 0755); err != nil {
		return fmt.Errorf("create .sindri/agents: %w", err)
	}

	cfgPath := filepath.Join(Dir(projectRoot), "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		data, _ := json.MarshalIndent(Config{Version: configVersion}, "", "  ")
		if err := os.WriteFile(cfgPath, append(data, '\n'), 0644); err != nil {
			return fmt.Errorf("write config.json: %w", err)
		}
	}

	// Seed the singleton reviewer. Every project has exactly one reviewer,
	// bound to the main repo, so it is registered at scaffold time — that is
	// what lets List() read its role from the index instead of inferring it
	// from position.
	if _, err := os.Stat(agentPath(projectRoot, "reviewer")); os.IsNotExist(err) {
		if err := WriteAgent(projectRoot, Agent{Name: "reviewer", Role: "reviewer"}); err != nil {
			return err
		}
	}

	return ensureGitignore(projectRoot)
}

// ensureGitignore appends `.sindri/` to the project's `.gitignore` if missing.
func ensureGitignore(projectRoot string) error {
	path := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == ".sindri/" {
			return nil
		}
	}
	out := string(data)
	if len(out) > 0 && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += ".sindri/\n"
	if err := os.WriteFile(path, []byte(out), 0644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

// WriteAgent writes (or replaces) an agent's index entry. The agents directory
// is created on demand, so callers need not call EnsureSindri first. CreatedAt
// is preserved across rewrites; it is stamped on first write.
func WriteAgent(projectRoot string, a Agent) error {
	if err := os.MkdirAll(AgentsDir(projectRoot), 0755); err != nil {
		return fmt.Errorf("create .sindri/agents: %w", err)
	}
	if a.CreatedAt == "" {
		if prev, err := ReadAgent(projectRoot, a.Name); err == nil {
			a.CreatedAt = prev.CreatedAt
		}
	}
	if a.CreatedAt == "" {
		a.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent %s: %w", a.Name, err)
	}
	if err := os.WriteFile(agentPath(projectRoot, a.Name), append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write agent %s: %w", a.Name, err)
	}
	return nil
}

// ReadAgent reads a single agent's index entry.
func ReadAgent(projectRoot, name string) (Agent, error) {
	var a Agent
	data, err := os.ReadFile(agentPath(projectRoot, name))
	if err != nil {
		return a, err
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return a, fmt.Errorf("parse agent %s: %w", name, err)
	}
	return a, nil
}

// Roster returns every agent in the index, sorted by name. This directory
// listing is the canonical set of agents that should exist.
func Roster(projectRoot string) ([]Agent, error) {
	entries, err := os.ReadDir(AgentsDir(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var agents []Agent
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		a, err := ReadAgent(projectRoot, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skip bad index entry %s: %v\n", e.Name(), err)
			continue
		}
		agents = append(agents, a)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents, nil
}
