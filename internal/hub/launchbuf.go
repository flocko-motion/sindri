// package: hub / launchbuf
// type:    logic (launch output capture)
// job:     a per-agent, concurrency-safe buffer that Launch tees the image-build
//          and pod-start output into, so the TUI's live-screen region can show
//          launch progress while the pod is still coming up (AgentPane returns
//          its tail until the container has logs of its own).
package hub

import (
	"bytes"
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

// newLaunchBuf installs a fresh launch-output buffer for an agent and returns it.
func (h *Hub) newLaunchBuf(name string) *safeBuffer {
	b := &safeBuffer{}
	h.launchMu.Lock()
	h.launchBuf[name] = b
	h.launchMu.Unlock()
	return b
}

// launchOutput returns the tail of an agent's captured launch output ("" if none).
func (h *Hub) launchOutput(name string) string {
	h.launchMu.Lock()
	defer h.launchMu.Unlock()
	if b, ok := h.launchBuf[name]; ok {
		return b.tail(200)
	}
	return ""
}
