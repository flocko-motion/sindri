package claude

import (
	"testing"

	"github.com/flo-at/sindri/internal/adapter/agent"
)

func TestClaudeState(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   agent.State
	}{
		{
			name:   "permission prompt is blocked",
			screen: "Do you want to proceed?\n❯ 1. Yes\n  2. No\n(esc to cancel)",
			want:   agent.Blocked,
		},
		{
			name:   "selection form is blocked",
			screen: "Pick a plan\n> option a\n  option b\n\nenter to select · arrow keys to navigate · esc to cancel",
			want:   agent.Blocked,
		},
		{
			name:   "would-you-like with yes/no is blocked",
			screen: "Would you like to continue?\n❯ Yes\n  No",
			want:   agent.Blocked,
		},
		{
			name:   "interrupt hint is working",
			screen: "✳ Cooking… (esc to interrupt)\n  · running tests",
			want:   agent.Working,
		},
		{
			name:   "bare prompt box is idle",
			screen: "some earlier output\n\n╭─────────────╮\n❯                          \n╰─────────────╯",
			want:   agent.Idle,
		},
		{
			name:   "transcript viewer is unknown (state hidden)",
			screen: "showing detailed transcript\nctrl+o to toggle · ↑↓ scroll",
			want:   agent.Unknown,
		},
		{
			name:   "plain shell is unknown",
			screen: "sindri@austri:/workspace$ ls\nREADME.md  go.mod",
			want:   agent.Unknown,
		},
		{
			name:   "blocked wins over the visible prompt box",
			screen: "Do you want to proceed?\n❯ 1. Yes\n  2. No\n(esc to cancel)\n❯ ",
			want:   agent.Blocked,
		},
	}
	for _, c := range cases {
		if got := (Claude{}).DetectState(c.screen); got != c.want {
			t.Errorf("%s: DetectState() = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestRuntime pins the shared classifier the board and the herdr sidebar both read
// through — working/blocked/idle, with an unrecognized screen counting as idle.
func TestRuntime(t *testing.T) {
	agent.Use(New())
	cases := []struct {
		name   string
		screen string
		want   string
	}{
		{"interrupt hint is working", "✳ Cooking… (esc to interrupt)\n  · running tests", "working"},
		{"permission prompt is blocked", "Do you want to proceed?\n❯ 1. Yes\n  2. No\n(esc to cancel)", "blocked"},
		{"bare prompt box is idle", "╭─────────────╮\n❯                          \n╰─────────────╯", "idle"},
		{"unrecognized shell counts as idle", "sindri@austri:/workspace$ ls\nREADME.md  go.mod", "idle"},
	}
	for _, c := range cases {
		if got := agent.Runtime(c.screen); got != c.want {
			t.Errorf("%s: Runtime() = %q, want %q", c.name, got, c.want)
		}
	}
}
