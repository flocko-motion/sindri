package client

import (
	"os/exec"
	"testing"
	"time"
)

// TestProcessAliveRejectsZombie guards the restart-wedging bug: a hub that died
// but was never reaped by its parent lingers as a zombie. It still answers signal
// 0 (kill 0 succeeds), so the old check counted it as alive and refused every
// restart. ProcessAlive must treat it as dead.
func TestProcessAliveRejectsZombie(t *testing.T) {
	// A child that exits immediately and is never Wait()ed becomes a zombie.
	c := exec.Command("true")
	if err := c.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	pid := c.Process.Pid
	defer c.Wait() // reap it at the end

	var sawZombie bool
	for i := 0; i < 200; i++ { // up to ~2s for it to exit into zombie state
		if isZombie(pid) {
			sawZombie = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sawZombie {
		t.Skip("could not observe a zombie state on this platform")
	}
	if ProcessAlive(pid) {
		t.Fatal("ProcessAlive counted a zombie as alive — a dead hub would wedge every restart")
	}
}
