package tmux

import (
	"slices"
	"testing"
)

func TestSendTextIsLiteralThenEnter(t *testing.T) {
	cmds := SendText("brokkr", "[user] hello; rm -rf /")
	if len(cmds) != 2 {
		t.Fatalf("want 2 argvs, got %d", len(cmds))
	}
	// First argv must carry the -l -- literal markers immediately before the text,
	// so a provenance tag or shell metacharacters are never interpreted.
	want := []string{"send-keys", "-t", "brokkr", "-l", "--", "[user] hello; rm -rf /"}
	if !slices.Equal(cmds[0], want) {
		t.Fatalf("literal argv wrong:\n got %q\nwant %q", cmds[0], want)
	}
	if !slices.Equal(cmds[1], []string{"send-keys", "-t", "brokkr", "Enter"}) {
		t.Fatalf("enter argv wrong: %q", cmds[1])
	}
}

func TestAttachReadOnly(t *testing.T) {
	// Read-write attach carries -d so it evicts any other (incl. orphaned) client
	// and the human gets sole control.
	if got := Attach("brokkr", false); !slices.Equal(got, []string{"attach-session", "-t", "brokkr", "-d"}) {
		t.Fatalf("attach: %q", got)
	}
	// Read-only observers use -r and must NOT carry -d — they watch without kicking
	// the actual driver.
	if got := Attach("brokkr", true); !slices.Equal(got, []string{"attach-session", "-t", "brokkr", "-r"}) {
		t.Fatalf("attach -r: %q", got)
	}
}

func TestListClients(t *testing.T) {
	want := []string{"list-clients", "-t", "brokkr", "-F", "#{client_tty} #{client_width} #{client_height} #{client_readonly}"}
	if got := ListClients("brokkr"); !slices.Equal(got, want) {
		t.Fatalf("list-clients argv:\n got %q\nwant %q", got, want)
	}
}

func TestCapturePane(t *testing.T) {
	// Plain (color=false): the visible screen only, no escape sequences — this dump
	// is pattern-matched to classify Claude's state, so colour would corrupt it.
	if got := CapturePane("brokkr", 0, false); !slices.Equal(got, []string{"capture-pane", "-t", "brokkr", "-p"}) {
		t.Fatalf("plain capture argv: %q", got)
	}
	// Coloured (color=true): -e keeps the pane's escape sequences for the TUI
	// preview, which renders ANSI.
	if got := CapturePane("brokkr", 0, true); !slices.Equal(got, []string{"capture-pane", "-t", "brokkr", "-p", "-e"}) {
		t.Fatalf("colour capture argv: %q", got)
	}
	// lines>0 reaches that many rows back into the scrollback (-S -<lines>), after
	// the -e flag.
	if got := CapturePane("brokkr", 40, true); !slices.Equal(got, []string{"capture-pane", "-t", "brokkr", "-p", "-e", "-S", "-40"}) {
		t.Fatalf("colour+scrollback capture argv: %q", got)
	}
}
