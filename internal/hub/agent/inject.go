// package: hub/agent / inject
// type:    logic (message delivery into a running agent)
// job:     the mechanics of putting text into an agent's live tmux session (via the
// container runtime) — Inject, InjectWhenReady (wait briefly, for messages
// right after a launch), and Tell (a source-stamped, logged message). WHAT
// to send and WHEN is the caller's; this just delivers.
// limits:  no decision-making; the tmux session is named after the agent.
package agent

import (
	"context"
	"fmt"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/container"
)

// Inject types text into an agent's tmux session via the container runtime. Fails if the agent isn't
// running (the caller decides whether that's an error or to wait), or if it is signed out: text
// typed at that prompt is never sent, it piles up in the input box unread.
func (s *Service) Inject(project, name, text string) error {
	c := s.deps.ContainerName(project, name)
	if !container.Running(c) {
		return fmt.Errorf("agent %q is not running — launch it first", name)
	}
	if s.RuntimeState(context.Background(), project, name) == string(agentport.SignedOut) {
		return fmt.Errorf("agent %q is signed out — its pane says to run /login, and nothing typed there is sent. "+
			"Its credentials come from the host and the hub keeps them staged, so the running process just has to "+
			"re-read them: `sindri agent restart %s` (the session resumes). If the host is signed out too, log in "+
			"there first", name, name)
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
				err := s.Inject(project, name, text)
				if err != nil {
					// Recorded, not dropped: the reason belongs to the agent, and the log is where a
					// user reconstructs what it was never told.
					_ = s.store.For(project).Log(name, "inject-skipped", text)
				}
				return err
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return s.store.For(project).Log(name, "inject-skipped", text)
}

// Interrupt sends an Escape to the agent's tmux session — Claude's "abort the current
// operation" key — so a follow-up message lands on an idle prompt instead of being
// queued behind in-flight work. Errors when the agent isn't running; the caller
// decides whether that matters (for a scrap it's fine — nothing to interrupt).
func (s *Service) Interrupt(project, name string) error {
	c := s.deps.ContainerName(project, name)
	if !container.Running(c) {
		return fmt.Errorf("agent %q is not running", name)
	}
	full := append([]string{"tmux"}, tmux.Interrupt(name)...)
	_, err := container.Exec(c, full...)
	return err
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
