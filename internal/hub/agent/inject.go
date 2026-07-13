// package: hub/agent / inject
// type:    logic (message delivery into a running agent)
// job:     the mechanics of putting text into an agent's live tmux session (via the
//          container runtime) — Inject, InjectWhenReady (wait briefly, for messages
//          right after a launch), and Tell (a source-stamped, logged message). WHAT
//          to send and WHEN is the caller's; this just delivers.
// limits:  no decision-making; the tmux session is named after the agent.
package agent

import (
	"fmt"
	"time"

	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/container"
)

// Inject types text into an agent's tmux session via the container runtime. Fails if
// the agent isn't running (the caller decides whether that's an error or to wait).
func (s *Service) Inject(project, name, text string) error {
	c := s.deps.ContainerName(project, name)
	if !container.Running(c) {
		return fmt.Errorf("agent %q is not running — launch it first", name)
	}
	for _, argv := range tmux.SendText(name, text) { // the tmux session is the agent name
		full := append([]string{"tmux"}, argv...)
		if _, err := container.Exec(c, full...); err != nil {
			return err
		}
	}
	return nil
}

// InjectWhenReady waits (briefly) for an agent's tmux session to exist, then injects.
// Used for hub-originated messages (verdicts, rehydrate) right after a launch, when
// the session may not be up yet. A message that never lands is recorded so it is not
// silently lost.
func (s *Service) InjectWhenReady(project, name, text string) error {
	c := s.deps.ContainerName(project, name)
	for i := 0; i < 25; i++ {
		if container.Running(c) {
			if _, err := container.Exec(c, "tmux", "has-session", "-t", name); err == nil {
				return s.Inject(project, name, text)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return s.store.For(project).Log(name, "inject-skipped", text)
}

// Tell delivers a message into an agent's session, stamped with its source
// (provenance, D12). The stamped line is recorded in the activity log.
func (s *Service) Tell(project, name, msg, source string) error {
	ps := s.store.For(project)
	if _, ok, err := ps.GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if source == "" {
		source = "user"
	}
	stamped := fmt.Sprintf("[%s] %s", source, msg)
	if err := s.Inject(project, name, stamped); err != nil {
		return err
	}
	defer s.deps.Notify()
	return ps.Log(name, "recv", stamped)
}
