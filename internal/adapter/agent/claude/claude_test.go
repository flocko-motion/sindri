package claude

import (
	"strings"
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
		{
			// eitri's pane, verbatim: a hub message sat unsent IN the input box while the board read
			// "idle" for hours. The box is drawn, so signed-out has to be decided before idle.
			name:   "expired login outranks the prompt box",
			screen: "❯ [hub] feat-macos-release moved — your branch was rebased onto it.\n\n● Login expired · Please run /login\n\n────────\n❯ \n────────",
			want:   agent.SignedOut,
		},
		{
			name:   "an invalid key is the same banner",
			screen: "● Invalid API key · Please run /login\n❯ ",
			want:   agent.SignedOut,
		},
		{
			// The pattern lives in a file sindri's own agents edit. Matching the words wherever they
			// appear would have every pane showing this source read as an agent that cannot work.
			name:   "the banner quoted in source text is not the state",
			screen: "✳ Editing… (esc to interrupt)\n  regexp.MustCompile(`· please run /login$`) // the banner",
			want:   agent.Working,
		},
		{
			// eitri, one minute after it was logged back in: the banner was still on screen while it
			// answered the user below it. An interrupt hint is happening NOW; the banner may be history.
			name: "a recovered agent working below an old banner is working",
			screen: "● Login expired · Please run /login\n" +
				strings.Repeat("  more output since then\n", 14) + "✳ Crunching… (esc to interrupt)\n❯ ",
			want: agent.Working,
		},
		{
			// Same pane at rest: the banner has scrolled out of the live region, so what is true now is
			// an idle prompt. Read as signed-out it would have sent the user to fix a working agent.
			name: "a banner scrolled out of the live region is history",
			screen: "● Login expired · Please run /login\n" +
				strings.Repeat("  more output since then\n", 14) + "❯ ",
			want: agent.Idle,
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
