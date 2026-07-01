package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// seedPID writes a raw pid file for root (creating .sindri), for tests that need a
// pre-existing owner.
func seedPID(t *testing.T, root string, pid int, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".sindri"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"pid":%d,"version":%q}`, pid, version)
	if err := os.WriteFile(pidPath(root), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPIDFileRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, _, ok := ReadPID(root); ok {
		t.Fatal("no pid file yet, but ReadPID reported ok")
	}
	if err := WritePID(root, "1.2.3"); err != nil {
		t.Fatalf("WritePID: %v", err)
	}
	pid, ver, ok := ReadPID(root)
	if !ok || pid != os.Getpid() || ver != "1.2.3" {
		t.Fatalf("ReadPID = (%d, %q, %v), want (%d, \"1.2.3\", true)", pid, ver, ok, os.Getpid())
	}
	// A re-stamp by the same process is allowed (idempotent).
	if err := WritePID(root, "1.2.4"); err != nil {
		t.Fatalf("re-stamp by the same process should be allowed: %v", err)
	}
	RemovePID(root)
	if _, _, ok := ReadPID(root); ok {
		t.Fatal("RemovePID left a readable pid file")
	}
}

func TestWritePIDOverwritesStale(t *testing.T) {
	root := t.TempDir()
	// A dead owner (a huge pid is almost certainly free) must not block a new hub.
	seedPID(t, root, 2147483000, "old")
	if err := WritePID(root, "new"); err != nil {
		t.Fatalf("WritePID should overwrite a stale file: %v", err)
	}
	if _, ver, _ := ReadPID(root); ver != "new" {
		t.Fatalf("version = %q, want \"new\"", ver)
	}
}

func TestWritePIDRefusesLiveOwner(t *testing.T) {
	root := t.TempDir()
	// The parent process is alive and isn't us — a stand-in for another running hub.
	seedPID(t, root, os.Getppid(), "other")
	if err := WritePID(root, "mine"); err == nil {
		t.Fatal("WritePID should refuse when a different live process owns the repo")
	}
	if _, ver, _ := ReadPID(root); ver != "other" {
		t.Fatalf("version = %q, want the owner untouched (\"other\")", ver)
	}
}
