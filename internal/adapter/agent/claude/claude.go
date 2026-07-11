// package: adapter/agent/claude / claude
// type:    adapter (Claude Code — implements adapter/agent.Agent)
// job:     the Claude Code backend of the coding-agent port: classify a Claude pane's
//          live runtime state (working/blocked/idle/unknown) from its rendered screen
//          text. Patterns and precedence borrowed from herdr's claude.toml so sindri
//          reports the same runtime states herdr's sidebar shows.
// limits:  Claude Code only. Pure text classification, no I/O — the caller supplies
//          the captured pane (tmux capture-pane).
package claude

import (
	"regexp"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/agent"
)

// Claude implements agent.Agent for Claude Code.
type Claude struct{}

// New returns the Claude coding-agent backend.
func New() agent.Agent { return Claude{} }

var (
	// promptLine matches Claude's empty input box (the ❯ chevron at line start).
	promptLine = regexp.MustCompile(`(?m)^\s*❯`)
	// yesNoOption matches a selectable yes/no line ("❯ 1. Yes", "2. No", "❯ Yes").
	yesNoOption = regexp.MustCompile(`(?im)^\s*(❯\s*)?(\d+\.\s*)?(yes|no)\b`)
)

// DetectState reads a Claude Code pane's rendered screen text (as `tmux
// capture-pane -p` yields) into a runtime state. Precedence mirrors herdr's
// claude.toml: a hidden transcript view is unknown; a response prompt is blocked;
// the interrupt hint is working; a bare prompt box is idle. Case-insensitive.
func (Claude) DetectState(screen string) agent.State {
	s := strings.ToLower(screen)
	has := func(subs ...string) bool { // every substring present
		for _, sub := range subs {
			if !strings.Contains(s, sub) {
				return false
			}
		}
		return true
	}

	// A transcript/history viewer hides the live prompt — the real state is unknown.
	if has("showing detailed transcript") {
		return agent.Unknown
	}

	// Blocked: Claude is waiting on a human response. Checked before idle because a
	// prompt box (❯) is also visible while blocked.
	switch {
	case has("enter to select", "esc to cancel") && hasNav(s):
		return agent.Blocked // a selection form
	case has("run a dynamic workflow?", "esc to cancel"):
		return agent.Blocked
	case has("do you want to proceed?"):
		return agent.Blocked // permission / confirmation prompt
	case has("waiting for permission"):
		return agent.Blocked
	case (has("do you want to") || has("would you like to")) &&
		(strings.Contains(s, "❯") || yesNoOption.MatchString(screen)):
		return agent.Blocked
	}

	// Working: Claude shows its interrupt hint while a turn runs.
	if has("esc to interrupt") {
		return agent.Working
	}

	// Idle: the empty prompt box is visible and nothing above needs an answer.
	if promptLine.MatchString(screen) {
		return agent.Idle
	}

	return agent.Unknown
}

// hasNav reports whether the (lowercased) screen shows a navigation hint of the kind
// Claude renders under a selection form.
func hasNav(s string) bool {
	for _, hint := range []string{
		"arrow keys to navigate", "tab/arrow keys", "arrows to navigate",
		"↑/↓ to navigate", "↑↓ to navigate",
	} {
		if strings.Contains(s, hint) {
			return true
		}
	}
	return false
}
