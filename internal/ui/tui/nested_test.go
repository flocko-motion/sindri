package tui

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// livePID starts a process that outlives the assertion and returns its pid.
func livePID(t *testing.T) int {
	t.Helper()
	c := exec.Command("sleep", "60")
	if err := c.Start(); err != nil {
		t.Skipf("cannot start a helper process: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
	})
	return c.Process.Pid
}

// deadPID returns the pid of a process that has exited AND been reaped, so nothing is left in the
// process table — the stale marker a TUI leaves behind when it dies in a shell it opened.
func deadPID(t *testing.T) int {
	t.Helper()
	c := exec.Command("true")
	if err := c.Run(); err != nil { // Run waits, so it is reaped, not a zombie
		t.Skipf("cannot run a helper process: %v", err)
	}
	return c.Process.Pid
}

// TestParentTUIDetectsALiveOne: a marker naming a running process means we are in that TUI's
// shell, which is the case the guard exists for.
func TestParentTUIDetectsALiveOne(t *testing.T) {
	want := livePID(t)
	t.Setenv(tuiPIDEnv, strconv.Itoa(want))
	pid, nested := ParentTUI()
	if !nested || pid != want {
		t.Errorf("ParentTUI() = (%d, %v), want (%d, true)", pid, nested, want)
	}
}

// TestParentTUIIgnoresAStaleMarker is why the marker carries a pid at all: a flag left behind by a
// TUI that died here would refuse every TUI in this terminal from then on, unanswerably.
func TestParentTUIIgnoresAStaleMarker(t *testing.T) {
	t.Setenv(tuiPIDEnv, strconv.Itoa(deadPID(t)))
	if pid, nested := ParentTUI(); nested {
		t.Errorf("a dead pid must not count as a parent, got (%d, %v)", pid, nested)
	}
}

// TestParentTUIIgnoresItself: the TUI exports the marker for its children and inherits it, so its
// own pid must never read as a parent — that would refuse the very TUI that set it.
func TestParentTUIIgnoresItself(t *testing.T) {
	t.Setenv(tuiPIDEnv, strconv.Itoa(os.Getpid()))
	if pid, nested := ParentTUI(); nested {
		t.Errorf("our own pid must not count as a parent, got (%d, %v)", pid, nested)
	}
}

// TestParentTUIIgnoresNonsense: an unset or unparseable marker is not a claim to check.
func TestParentTUIIgnoresNonsense(t *testing.T) {
	for _, v := range []string{"", "abc", "0", "-1", "12.5", " "} {
		t.Setenv(tuiPIDEnv, v)
		if pid, nested := ParentTUI(); nested {
			t.Errorf("marker %q must be ignored, got (%d, %v)", v, pid, nested)
		}
	}
}

// TestMarkedCarriesOurPID: both doors out of the TUI (shell, editor) must carry the marker, and
// the editor's "nothing installed" nil has to survive being marked.
func TestMarkedCarriesOurPID(t *testing.T) {
	want := tuiPIDEnv + "=" + strconv.Itoa(os.Getpid())
	for _, c := range []*exec.Cmd{shellAt(t.TempDir()), marked(exec.Command("true"))} {
		if c == nil {
			t.Fatal("expected a command")
		}
		var found bool
		for _, kv := range c.Env {
			found = found || kv == want
		}
		if !found {
			t.Errorf("child is missing %q", want)
		}
	}
	if marked(nil) != nil {
		t.Error("marked(nil) must stay nil — editorAt reports 'no editor' that way")
	}
}

// TestRunRefusesInsideATUIShell: the whole point, end to end — Run declines before it touches a
// hub, and says how to get back rather than just failing.
func TestRunRefusesInsideATUIShell(t *testing.T) {
	t.Setenv(tuiPIDEnv, strconv.Itoa(livePID(t)))
	err := Run(t.TempDir())
	if err == nil {
		t.Fatal("expected Run to refuse inside a TUI's shell")
	}
	for _, want := range []string{"already running", "exit"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should mention %q, got: %v", want, err)
		}
	}
}
