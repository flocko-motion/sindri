package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"gopkg.in/yaml.v3"
)

// yamlKeys is the set of keys Config can hold, read off the struct rather than listed here —
// listing them is exactly the duplication that let Write drift from Config in the first place.
// Nested blocks are returned as "github.issues" style paths.
func yamlKeys(t reflect.Type, prefix string) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if tag == "" || tag == "-" { // untagged or deliberately not persisted (ArchitectureSet)
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			out = append(out, yamlKeys(ft, prefix+tag+".")...)
			continue
		}
		out = append(out, prefix+tag)
	}
	return out
}

// flatten turns the written YAML back into the same dotted-path key set.
func flatten(m map[string]any, prefix string) map[string]bool {
	out := map[string]bool{}
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			for kk := range flatten(sub, prefix+k+".") {
				out[kk] = true
			}
			continue
		}
		out[prefix+k] = true
	}
	return out
}

// TestWritePersistsEveryConfigKey is the guard on a silent wipe: Write rewrites the whole file from
// the struct it is given, so any key it forgets to emit is DELETED from a user's config on the next
// save — no error, no diff, just gone. `verify` was lost this way, and losing it removes a repo's
// submit gate for the whole fleet. Deriving the expectation from the struct means a key added to
// Config tomorrow fails here until Write learns to persist it.
func TestWritePersistsEveryConfigKey(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"ARCH.md", "Containerfile", "prompt.md", "verify.sh"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	on, maxLines, maxAvg := true, 400, 3.5
	full := Config{
		Architecture: "ARCH.md", Containerfile: "Containerfile", ReviewPrompt: "prompt.md",
		Verify: "verify.sh", Reference: "trunk", Reading: []string{"ARCH.md"},
		Lint:   api.Lint{MaxLines: &maxLines, MaxCommentAvg: &maxAvg},
		GitHub: api.GitHub{Issues: &on},
	}
	if err := Write(root, full); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".sindri", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	written := flatten(raw, "")
	for _, k := range yamlKeys(reflect.TypeOf(Config{}), "") {
		if !written[k] {
			t.Errorf("Write dropped %q — a save would delete it from the user's config:\n%s", k, data)
		}
	}
}

// TestWriteRoundTripsThroughLoad closes the loop the front-ends actually run: read the config, change
// one key, write it back. Every other key must survive the trip.
func TestWriteRoundTripsThroughLoad(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"ARCHITECTURE.md", "verify.sh"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".sindri"), 0o755); err != nil {
		t.Fatal(err)
	}
	const original = "verify: verify.sh\nreference: trunk\nreading:\n  - ARCHITECTURE.md\nlint:\n  max_lines: 400\n"
	if err := os.WriteFile(filepath.Join(root, ".sindri", "config.yaml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got.ReviewPrompt = "" // the edit: clearing an unrelated key must not disturb the rest
	if err := Write(root, got); err != nil {
		t.Fatalf("write: %v", err)
	}
	back, err := Load(root)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if back.Verify != "verify.sh" {
		t.Errorf("verify lost: %q — the repo's submit gate would silently disappear", back.Verify)
	}
	if back.Reference != "trunk" {
		t.Errorf("reference lost: %q", back.Reference)
	}
	if len(back.Reading) != 1 || back.Reading[0] != "ARCHITECTURE.md" {
		t.Errorf("reading lost: %v", back.Reading)
	}
	if back.Lint.MaxLines == nil || *back.Lint.MaxLines != 400 {
		t.Errorf("lint.max_lines lost: %v", back.Lint.MaxLines)
	}
}
