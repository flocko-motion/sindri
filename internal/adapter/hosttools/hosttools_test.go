package hosttools

import (
	"context"
	"testing"
)

// With no tools on PATH, Versions degrades to an empty map rather than an error — the shape
// toolskew.check() needs from a host that lacks one or more of the tools it compares.
func TestVersionsNoToolsOnPath(t *testing.T) {
	t.Setenv("PATH", "")
	got := Versions(context.Background())
	if len(got) != 0 {
		t.Errorf("no tools on PATH, want an empty map, got %v", got)
	}
}

// The actual parsing logic, pinned without needing go on PATH: "go version X Y" -> "X".
func TestParseGoVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"normal", "go version go1.27.0 linux/amd64", "go1.27.0", true},
		{"trailing newline", "go version go1.27.0 linux/amd64\n", "go1.27.0", true},
		{"too short", "go version", "", false},
		{"empty", "", "", false},
	}
	for _, c := range cases {
		got, ok := parseGoVersion(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: parseGoVersion(%q) = (%q, %v), want (%q, %v)", c.name, c.in, got, ok, c.want, c.ok)
		}
	}
}

// The actual parsing logic for brokkr's "brokkr <version>" first line, pinned without needing a
// real `brokkr` on PATH.
func TestParseBrokkrVersion(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"normal", "brokkr v1.2.3-4-gabcdef", "v1.2.3-4-gabcdef"},
		{"dev build", "brokkr dev", "dev"},
		{"no version", "brokkr", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		got, ok := parseBrokkrVersion(c.in)
		wantOK := c.want != ""
		if ok != wantOK || got != c.want {
			t.Errorf("%s: parseBrokkrVersion(%q) = (%q, %v), want (%q, %v)", c.name, c.in, got, ok, c.want, wantOK)
		}
	}
}

// A CLI whose --version prints more than one line must compare against only its first: the pod side
// (-> container.ParseVersionManifest) only ever captures the single build-log line its marker echo
// produced, so keeping the host's trailing banner lines would make every such tool a permanent,
// unactionable mismatch.
func TestFirstLine(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"single line", "1.8.0", "1.8.0"},
		{"trailing newline", "1.8.0\n", "1.8.0"},
		{"banner after", "1.8.0\nsome extra banner line\n", "1.8.0"},
		{"padded first line", "  1.8.0  \nEXTRA", "1.8.0"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := firstLine(c.in); got != c.want {
			t.Errorf("%s: firstLine(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
