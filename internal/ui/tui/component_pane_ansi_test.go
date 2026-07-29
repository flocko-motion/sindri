package tui

import (
	"strings"
	"testing"
)

// TestKeepColourDropsFrameCommands: a captured tmux pane is another program's full-screen
// drawing. Its cursor moves, erases and scroll regions would execute against OUR frame,
// wiping rows that belong to the layout — the header appearing to vanish mid-render.
func TestKeepColourDropsFrameCommands(t *testing.T) {
	for _, seq := range []string{
		"\x1b[2J",     // erase display — clears our screen
		"\x1b[H",      // cursor home
		"\x1b[K",      // erase to end of line
		"\x1b[3;10H",  // absolute cursor position
		"\x1b[1;40r",  // scroll region
		"\x1b[?25l",   // hide cursor
		"\x1b[2A",     // cursor up
		"\x1b]0;ttl\x07", // OSC title
		"\x1b(B",      // charset
		"\x1bM",       // reverse index
		"\r",          // bleeds over the left edge
		"\x07",        // bell
	} {
		got := KeepColour("before" + seq + "after")
		if got != "beforeafter" {
			t.Errorf("sequence %q survived: %q", seq, got)
		}
	}
}

// TestKeepColourKeepsSGR: colour is presentation, not a command, and the pane preview is
// worth reading in colour.
func TestKeepColourKeepsSGR(t *testing.T) {
	in := "\x1b[31mred\x1b[0m \x1b[1;32mbold green\x1b[m plain"
	got := KeepColour(in)
	if got != in {
		t.Errorf("SGR must survive:\n got %q\nwant %q", got, in)
	}
	// Newlines are structure and must survive; the caller splits on them.
	if got := KeepColour("a\nb"); got != "a\nb" {
		t.Errorf("newlines must survive, got %q", got)
	}
}

// TestKeepColourRealisticPane: a mixed line keeps its words and colour and loses the rest.
func TestKeepColourRealisticPane(t *testing.T) {
	in := "\x1b[?25l\x1b[H\x1b[2J\x1b[36m● Running…\x1b[0m\x1b[K\r\n\x1b[2muser@host\x1b[0m"
	got := KeepColour(in)
	for _, want := range []string{"● Running…", "user@host", "\x1b[36m", "\x1b[2m"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q from %q", want, got)
		}
	}
	for _, bad := range []string{"\x1b[H", "\x1b[2J", "\x1b[K", "\x1b[?25l", "\r"} {
		if strings.Contains(got, bad) {
			t.Errorf("kept %q in %q", bad, got)
		}
	}
}
