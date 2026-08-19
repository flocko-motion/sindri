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
			// gloin, verbatim: the API cut the turn off, and the footer went on advertising a live turn.
			// Measured on it, the pane's digest still changed every few seconds — so neither the words
			// nor a still screen could see this, which is why it is matched by name.
			name:   "a cut-off turn outranks the interrupt hint it leaves behind",
			screen: "● API Error: Response stalled mid-stream. The response above may be incomplete.\n\n❯ \n  ⏵⏵ bypass permissions on (shift+tab to cycle) · esc to interrupt · ← for agents",
			want:   agent.Failed,
		},
		{
			// Once it really resumes, the error scrolls up out of the live region and the pane is a
			// working pane again. Matched anywhere, it would keep reporting a turn that already retried.
			name: "an API error scrolled out of the live region is history",
			screen: "● API Error: Response stalled mid-stream.\n" +
				strings.Repeat("  output since the retry\n", 14) + "✳ Working… (esc to interrupt)",
			want: agent.Working,
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
		{
			// sudri, verbatim: six minutes into a turn, and the board said blocked. The agent had ASKED
			// the user something several screens earlier, and the input box is always drawn — so the
			// pair matched anywhere on the pane and outranked the interrupt hint below it. The word
			// then flipped back to working when the sentence scrolled off, and blocked again at the
			// next one: it was reporting the transcript's contents, not the agent's state.
			name: "a question the agent asked earlier is transcript, not a prompt",
			screen: "● What would you like to clarify? Happy to give more background.\n" +
				strings.Repeat("  work since then\n", 40) +
				"✶ Perusing… (6m 1s · ↓ 25.6k tokens)\n❯ \n  ⏵⏵ bypass permissions on · esc to interrupt",
			want: agent.Working,
		},
		{
			// The same question, still inside the 30-line live region: nothing here is a yes/no option
			// line, so the bare ❯ (the input box, always drawn) must not stand in for one. Fixed by
			// dropping that alternative, not by the transcript happening to scroll far enough away.
			name: "a question still in the live region is not a yes/no prompt",
			screen: "● What would you like to clarify? Happy to give more background.\n" +
				strings.Repeat("  work since then\n", 5) +
				"✶ Perusing… (6m 1s · ↓ 25.6k tokens)\n❯ \n  ⏵⏵ bypass permissions on · esc to interrupt",
			want: agent.Working,
		},
		{
			// bombur, verbatim: the row read blocked while the question was the user's OWN, still being
			// typed into the box. asks("do you want to") matches the ❯ line same as any prose would, but
			// nothing below is a yes/no option, so the case no longer fires — it lands on promptLine,
			// which must match an input box holding text same as an empty one.
			name:   "a user's own question on the input line is not a prompt",
			screen: "╭──────────────────────────────────────╮\n❯ how many times do you want to run the tests?\n╰──────────────────────────────────────╯",
			want:   agent.Idle,
		},
		{
			// The other half of the same rule: a real prompt lives directly above the input box, however
			// long the transcript above it is, so scoping the match must not lose it.
			name: "a real prompt under a long transcript is still blocked",
			screen: strings.Repeat("  earlier output\n", 60) +
				"Do you want to proceed?\n❯ 1. Yes\n  2. No\n(esc to cancel)",
			want: agent.Blocked,
		},
	}
	for _, c := range cases {
		if got := (Claude{}).DetectState(c.screen); got != c.want {
			t.Errorf("%s: DetectState() = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestToolRunning pins the footer signal a long, silent shell call needs: austri, mid-`make verify`,
// showed exactly this pane for the whole 27s+ run, and DetectState alone reads it as Working either
// way — ToolRunning is the only thing that says the stillness is a tool call, not a frozen turn.
func TestToolRunning(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   bool
	}{
		{
			name:   "a shell in flight is running",
			screen: "✳ Cooking… (esc to interrupt)\n❯ \n  ⏵⏵ bypass permissions on · 1 shell · esc to interrupt",
			want:   true,
		},
		{
			name:   "more than one shell is still running",
			screen: "✳ Cooking… (esc to interrupt)\n❯ \n  ⏵⏵ bypass permissions on · 2 shells · esc to interrupt",
			want:   true,
		},
		{
			name:   "an ordinary working footer with no shell is not",
			screen: "✳ Cooking… (esc to interrupt)\n❯ \n  ⏵⏵ bypass permissions on (shift+tab to cycle) · esc to interrupt",
			want:   false,
		},
		{
			name:   "an idle prompt is not",
			screen: "╭─────────────╮\n❯                          \n╰─────────────╯",
			want:   false,
		},
		{
			name: "a shell count in old transcript, scrolled out of the footer, is not",
			screen: "  earlier: ran with 1 shell\n" + strings.Repeat("  more output since then\n", 14) +
				"✳ Cooking… (esc to interrupt)\n❯ \n  ⏵⏵ bypass permissions on (shift+tab to cycle) · esc to interrupt",
			want: false,
		},
		{
			// The false positive a reviewer caught live: this line, describing the footer rather than
			// being it, sits in the live region with no middot fencing it either side.
			name:   "the pattern named in prose inside the live region is not the footer",
			screen: `toolCount matches Claude's tool-tally footer ("1 shell", "2 shells") for real`,
			want:   false,
		},
	}
	for _, c := range cases {
		if got := (Claude{}).ToolRunning(c.screen); got != c.want {
			t.Errorf("%s: ToolRunning() = %v, want %v", c.name, got, c.want)
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
