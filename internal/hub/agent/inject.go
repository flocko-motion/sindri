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
	"io"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
)

// Inject types text into an agent's tmux session. It fails on an agent that isn't running (the
// caller decides whether to wait) and on a signed-out one, whose input box swallows anything typed.
// That refusal is for messages with nobody present to notice; a user's own answers for itself (-> Tell).
func (s *Service) Inject(ctx context.Context, project, name, text string) error {
	return s.inject(ctx, project, name, text, true)
}

// inject is the delivery; guard refuses a signed-out pane. False where a human, asked, said send
// anyway: the pane is a prediction and theirs may be the better information.
func (s *Service) inject(ctx context.Context, project, name, text string, guard bool) error {
	c := s.deps.ContainerName(project, name)
	if !container.RunningContext(ctx, c) {
		return fmt.Errorf("agent %q is not running — launch it first", name)
	}
	if guard && s.readsSignedOut(ctx, project, name) {
		return fmt.Errorf("agent %q is signed out — its pane says to run /login, and nothing typed there is sent. "+
			"Its credentials come from the host and the hub keeps them staged, so the running process just has to "+
			"re-read them: `sindri agent restart %s` (the session resumes). If the host is signed out too, log in "+
			"there first", name, name)
	}
	for _, argv := range tmux.SendText(name, text) { // the tmux session is the agent name
		full := append([]string{"tmux"}, argv...)
		if _, err := container.ExecContext(ctx, c, full...); err != nil {
			return err
		}
	}
	return nil
}

// InjectWhenReady waits (briefly) for the tmux session, then injects. It ERRORS when nothing was
// injected — logging the skip used to read as delivered, and a mail row's "pushed" reads this outcome.
func (s *Service) InjectWhenReady(ctx context.Context, project, name, text string) error {
	return s.injectWhenReady(ctx, project, name, text, true)
}

// injectWhenReady is InjectWhenReady with the guard as the caller's choice: an agent restarted for
// this very reason must not be turned away by a pane that has yet to redraw.
func (s *Service) injectWhenReady(ctx context.Context, project, name, text string, guard bool) error {
	c := s.deps.ContainerName(project, name)
	for i := 0; i < 25; i++ {
		if container.RunningContext(ctx, c) {
			if _, err := container.ExecContext(ctx, c, "tmux", "has-session", "-t", name); err == nil {
				err := s.inject(ctx, project, name, text, guard)
				if err != nil {
					// Recorded, not dropped: the reason belongs to the agent, and the log is where a
					// user reconstructs what it was never told.
					_ = s.store.For(project).Log(name, "inject-skipped", text)
				}
				return err
			}
		}
		// The wait is the caller's to abandon: a cancelled request has nobody left to deliver to.
		select {
		case <-ctx.Done():
			_ = s.store.For(project).Log(name, "inject-skipped", text)
			return fmt.Errorf("agent %q: %w — nothing was injected", name, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	_ = s.store.For(project).Log(name, "inject-skipped", text)
	return fmt.Errorf("agent %q has no live session — nothing was injected", name)
}

// Interrupt sends Escape — Claude's "abort the current operation" — so a follow-up message lands on
// an idle prompt rather than queueing behind in-flight work. Errors on an agent that isn't running,
// which the caller may not mind (a scrap has nothing to interrupt).
func (s *Service) Interrupt(ctx context.Context, project, name string) error {
	c := s.deps.ContainerName(project, name)
	if !container.RunningContext(ctx, c) {
		return fmt.Errorf("agent %q is not running", name)
	}
	full := append([]string{"tmux"}, tmux.Interrupt(name)...)
	_, err := container.ExecContext(ctx, c, full...)
	return err
}

// Tell delivers a source-stamped message (provenance, D12), recording the line in the activity log.
// signedOut is the sender's answer to a signed-out pane (api.SignedOutRefuse / Restart / Send): the
// default refuses, the other two because the person typing may know the pane is stale. PUSH-ONLY by
// decision, not omission (-> workflow.Delivery): synchronous, so a failure reaches whoever typed it.
func (s *Service) Tell(ctx context.Context, project, name, msg, source, signedOut string) error {
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
	if err := s.deliver(ctx, project, name, stamped, signedOut); err != nil {
		return err
	}
	defer s.deps.Notify()
	return ps.Log(name, "recv", stamped)
}

// deliver puts the line in, honouring the sender's answer about a signed-out pane: the restart the
// refusal has always named is carried out rather than recommended. The answer applies only where the
// pane really reads signed out — given in advance, it must not bounce a session that turned out fine.
func (s *Service) deliver(ctx context.Context, project, name, stamped, signedOut string) error {
	switch signedOut {
	case api.SignedOutSend, api.SignedOutRestart:
		if !s.readsSignedOut(ctx, project, name) {
			break
		}
		if signedOut == api.SignedOutRestart {
			if err := s.RestartAgent(ctx, project, name, io.Discard); err != nil {
				return fmt.Errorf("restarting %s to deliver the message: %w", name, err)
			}
			return s.injectWhenReady(ctx, project, name, stamped, false)
		}
		return s.inject(ctx, project, name, stamped, false)
	}
	return s.Inject(ctx, project, name, stamped)
}

// readsSignedOut is the pane's verdict on whether anything typed is sent — an observation, which is
// why a user may overrule it and the hub may not.
func (s *Service) readsSignedOut(ctx context.Context, project, name string) bool {
	return s.RuntimeState(ctx, project, name) == string(agentport.SignedOut)
}
