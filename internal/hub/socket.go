// package: hub / socket
// type:    logic (transport addressing)
// job:     the per-repo control-socket path and a liveness probe, shared by the
//
//	server (bind) and clients (dial / ephemeral decision).
// limits:  path + liveness only; serving the socket (-> server.go) and the verb
//          set (-> commands.go) live elsewhere.
package hub

import (
	"net"
	"path/filepath"
	"time"
)

// SocketPath is the hub's control socket for a repo.
func SocketPath(root string) string {
	return filepath.Join(root, ".sindri", "hub.sock")
}

// IsRunning reports whether a hub is listening on the repo's socket.
func IsRunning(root string) bool {
	c, err := net.DialTimeout("unix", SocketPath(root), 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
