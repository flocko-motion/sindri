// package: detect / claude
// type:    logic (agent runtime-state heuristics)
// job:     classify a Claude Code pane's live runtime state — working, blocked
//          (waiting for user input), idle (stopped at the prompt), or unknown —
//          from its rendered screen text. Patterns and precedence are borrowed from
//          herdr's claude.toml so sindri reports the same runtime states herdr's
//          sidebar shows.
// limits:  Claude Code only (the sole agent sindri runs). Pure text classification,
//          no I/O — the caller supplies the captured pane (tmux capture-pane).
package detect

import (
	"regexp"
	"strings"
)

// State is a Claude pane's detected runtime state (distinct from sindri's workflow
// phase — this is what Claude itself is doing right now).
type State string

const (
	Working State = "working" // actively processing a turn
	Blocked State = "blocked" // waiting for a user response (permission / question / selection)
	Idle    State = "idle"    // stopped at the prompt, nothing happening
	Unknown State = "unknown" // not classifiable (plain shell, transcript viewer, boot, …)
)

var (
	// promptLine matches Claude's empty input box (the ❯ chevron at line start).
	promptLine = regexp.MustCompile(`(?m)^\s*❯`)
	// yesNoOption matches a selectable yes/no line ("❯ 1. Yes", "2. No", "❯ Yes").
	yesNoOption = regexp.MustCompile(`(?im)^\s*(❯\s*)?(\d+\.\s*)?(yes|no)\b`)
)

// ClaudeState classifies a Claude Code pane from its rendered screen text (as
// `tmux capture-pane -p` yields). Precedence mirrors herdr's claude.toml: a hidden
// transcript view is unknown; a response prompt is blocked; the interrupt hint is
// working; a bare prompt box is idle. Matching is case-insensitive.
func ClaudeState(screen string) State {
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
		return Unknown
	}

	// Blocked: Claude is waiting on a human response. Checked before idle because a
	// prompt box (❯) is also visible while blocked.
	switch {
	case has("enter to select", "esc to cancel") && hasNav(s):
		return Blocked // a selection form
	case has("run a dynamic workflow?", "esc to cancel"):
		return Blocked
	case has("do you want to proceed?"):
		return Blocked // permission / confirmation prompt
	case has("waiting for permission"):
		return Blocked
	case (has("do you want to") || has("would you like to")) &&
		(strings.Contains(s, "❯") || yesNoOption.MatchString(screen)):
		return Blocked
	}

	// Working: Claude shows its interrupt hint while a turn runs.
	if has("esc to interrupt") {
		return Working
	}

	// Idle: the empty prompt box is visible and nothing above needs an answer.
	if promptLine.MatchString(screen) {
		return Idle
	}

	return Unknown
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
