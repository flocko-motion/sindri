// package: hub/agent / launchbuf
// type:    logic (launch output capture)
// job:     a per-agent, concurrency-safe buffer that the hub's launch path tees the
//          image-build and pod-start output into, so the TUI's live-screen region can
//          show launch progress while the pod is still coming up (the agent pane
//          returns its tail until the container has logs of its own).
// limits:  just buffers bytes; deciding what to tee in is the launch path's and
//          showing it is the TUI's.
package agent

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// safeBuffer is an io.Writer whose contents can be read concurrently.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// tail returns the last n lines of the buffer.
func (b *safeBuffer) tail(n int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := strings.Split(strings.TrimRight(b.buf.String(), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// launchKey keys the per-agent launch buffers by (project, name).
func launchKey(project, name string) string { return project + "\x00" + name }

// NewLaunchBuf installs a fresh launch-output buffer for an agent and returns it as
// the io.Writer the launch path tees into.
func (s *Service) NewLaunchBuf(project, name string) io.Writer {
	b := &safeBuffer{}
	s.launchMu.Lock()
	s.launch[launchKey(project, name)] = b
	s.launchMu.Unlock()
	return b
}

// LaunchOutput returns the tail of an agent's captured launch output ("" if none).
func (s *Service) LaunchOutput(project, name string) string {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	if b, ok := s.launch[launchKey(project, name)]; ok {
		return b.tail(200)
	}
	return ""
}
