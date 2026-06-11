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
	if got := Attach("brokkr", false); !slices.Equal(got, []string{"attach-session", "-t", "brokkr"}) {
		t.Fatalf("attach: %q", got)
	}
	if got := Attach("brokkr", true); !slices.Equal(got, []string{"attach-session", "-t", "brokkr", "-r"}) {
		t.Fatalf("attach -r: %q", got)
	}
}
