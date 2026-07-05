package hub

import (
	"fmt"
	"os"
	"testing"
)

// tmpRuntime points the runtime dir (where the pid file lives) at a temp dir for
// the duration of a test.
func tmpRuntime(t *testing.T) {
	t.Helper()
	t.Setenv("SINDRI_HOME", t.TempDir())
}

// seedPID writes a raw pid file, for tests that need a pre-existing owner.
func seedPID(t *testing.T, pid int, version string) {
	t.Helper()
	body := fmt.Sprintf(`{"pid":%d,"version":%q}`, pid, version)
	if err := os.WriteFile(pidPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPIDFileRoundTrip(t *testing.T) {
	tmpRuntime(t)
	if _, _, ok := ReadPID(); ok {
		t.Fatal("no pid file yet, but ReadPID reported ok")
	}
	if err := WritePID("1.2.3"); err != nil {
		t.Fatalf("WritePID: %v", err)
	}
	pid, ver, ok := ReadPID()
	if !ok || pid != os.Getpid() || ver != "1.2.3" {
		t.Fatalf("ReadPID = (%d, %q, %v), want (%d, \"1.2.3\", true)", pid, ver, ok, os.Getpid())
	}
	// A re-stamp by the same process is allowed (idempotent).
	if err := WritePID("1.2.4"); err != nil {
		t.Fatalf("re-stamp by the same process should be allowed: %v", err)
	}
	RemovePID()
	if _, _, ok := ReadPID(); ok {
		t.Fatal("RemovePID left a readable pid file")
	}
}

func TestWritePIDOverwritesStale(t *testing.T) {
	tmpRuntime(t)
	if err := os.MkdirAll(pidDir(t), 0o755); err != nil {
		t.Fatal(err)
	}
	// A dead owner (a huge pid is almost certainly free) must not block a new hub.
	seedPID(t, 2147483000, "old")
	if err := WritePID("new"); err != nil {
		t.Fatalf("WritePID should overwrite a stale file: %v", err)
	}
	if _, ver, _ := ReadPID(); ver != "new" {
		t.Fatalf("version = %q, want \"new\"", ver)
	}
}

func TestWritePIDRefusesLiveOwner(t *testing.T) {
	tmpRuntime(t)
	if err := os.MkdirAll(pidDir(t), 0o755); err != nil {
		t.Fatal(err)
	}
	// The parent process is alive and isn't us — a stand-in for another running hub.
	seedPID(t, os.Getppid(), "other")
	if err := WritePID("mine"); err == nil {
		t.Fatal("WritePID should refuse when a different live process owns the hub")
	}
	if _, ver, _ := ReadPID(); ver != "other" {
		t.Fatalf("version = %q, want the owner untouched (\"other\")", ver)
	}
}

// pidDir is the directory holding the pid file (SINDRI_HOME, set by tmpRuntime).
func pidDir(t *testing.T) string {
	t.Helper()
	return os.Getenv("SINDRI_HOME")
}
