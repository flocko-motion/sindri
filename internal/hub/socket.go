// package: hub / socket
// type:    logic (transport addressing)
// job:     the global hub's single control-socket path and a liveness probe,
//          shared by the server (bind) and clients (dial). There is one hub for
//          the whole machine, so the socket lives under the runtime dir, not in
//          any repo.
// limits:  path + liveness only; serving the socket (-> server.go) and the verb
//          set (-> commands.go) live elsewhere.
package hub

import (
	"net"
	"path/filepath"
	"time"

	"github.com/flo-at/sindri/internal/tools/paths"
)

// SocketPath is the global hub's control socket.
func SocketPath() string {
	return filepath.Join(paths.RuntimeDir(), "hub.sock")
}

// IsRunning reports whether a hub is listening on the control socket.
func IsRunning() bool {
	c, err := net.DialTimeout("unix", SocketPath(), 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
