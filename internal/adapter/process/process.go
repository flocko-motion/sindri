// package: adapter/process / process
// type:    adapter (external tool: ps)
// job:     wraps `ps` for the process-table introspection the hub needs — telling a
// live pid from a zombie one. Single implementation, no swappable backend, so
// it is imported directly by rule; it still gets its own adapter package so no
// logic package shells out to `ps` itself.
// limits:  read-only process-state queries; starting/stopping processes is the caller's.
package process

import (
	"os/exec"
	"strconv"
	"strings"
)

// IsZombie reports whether pid is a zombie (defunct): still in the process table, holding its exit
// status until the parent reaps it, but dead. Best-effort — if `ps` can't be read we assume not a
// zombie, so a caller never discards a process that might still be live on a probe failure.
func IsZombie(pid int) bool {
	out, err := exec.Command("ps", "-o", "state=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}
