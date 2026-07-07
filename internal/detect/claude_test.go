package detect

import "testing"

func TestClaudeState(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   State
	}{
		{
			name:   "permission prompt is blocked",
			screen: "Do you want to proceed?\n❯ 1. Yes\n  2. No\n(esc to cancel)",
			want:   Blocked,
		},
		{
			name:   "selection form is blocked",
			screen: "Pick a plan\n> option a\n  option b\n\nenter to select · arrow keys to navigate · esc to cancel",
			want:   Blocked,
		},
		{
			name:   "would-you-like with yes/no is blocked",
			screen: "Would you like to continue?\n❯ Yes\n  No",
			want:   Blocked,
		},
		{
			name:   "interrupt hint is working",
			screen: "✳ Cooking… (esc to interrupt)\n  · running tests",
			want:   Working,
		},
		{
			name:   "bare prompt box is idle",
			screen: "some earlier output\n\n╭─────────────╮\n❯                          \n╰─────────────╯",
			want:   Idle,
		},
		{
			name:   "transcript viewer is unknown (state hidden)",
			screen: "showing detailed transcript\nctrl+o to toggle · ↑↓ scroll",
			want:   Unknown,
		},
		{
			name:   "plain shell is unknown",
			screen: "sindri@austri:/workspace$ ls\nREADME.md  go.mod",
			want:   Unknown,
		},
		{
			name:   "blocked wins over the visible prompt box",
			screen: "Do you want to proceed?\n❯ 1. Yes\n  2. No\n(esc to cancel)\n❯ ",
			want:   Blocked,
		},
	}
	for _, c := range cases {
		if got := ClaudeState(c.screen); got != c.want {
			t.Errorf("%s: ClaudeState() = %q, want %q", c.name, got, c.want)
		}
	}
}
