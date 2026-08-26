package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// PodBrokkrVersion must not error just because nothing has synced pod-bin yet — the normal state
// before any agent has ever launched, not a failure.
func TestPodBrokkrVersionNotYetSynced(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir())
	if v, ok := PodBrokkrVersion(); ok {
		t.Errorf("no pod-bin/brokkr yet, want ok=false, got %q", v)
	}
}

// A file at pod-bin/brokkr that isn't a real Go binary (a corrupted or partial copy) must degrade
// to ok=false, not panic or surface an error to the caller.
func TestPodBrokkrVersionNotABinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SINDRI_HOME", home)
	dir := filepath.Join(home, "pod-bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "brokkr"), []byte("not a binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if v, ok := PodBrokkrVersion(); ok {
		t.Errorf("a non-binary file should degrade to ok=false, got %q", v)
	}
}

// The actual parsing logic, pinned directly: brokkr's Makefile recipe stamps both main.version and
// main.buildTime into one -ldflags string, in either order, and only the version matters here.
func TestParseLdflagsVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"version only", "-X main.version=v1.2.3", "v1.2.3", true},
		{"version then buildTime", "-X main.version=v1.2.3-4-gabcdef -X main.buildTime=2026-08-25T12:00:00Z", "v1.2.3-4-gabcdef", true},
		{"buildTime then version", "-X main.buildTime=2026-08-25T12:00:00Z -X main.version=v1.2.3", "v1.2.3", true},
		{"no version flag", "-X main.buildTime=2026-08-25T12:00:00Z", "", false},
		{"empty", "", "", false},
	}
	for _, c := range cases {
		got, ok := parseLdflagsVersion(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: parseLdflagsVersion(%q) = (%q, %v), want (%q, %v)", c.name, c.in, got, ok, c.want, c.ok)
		}
	}
}
