package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
)

// TestConfigKeysCoverEverySettableKey: `repo config` is the CLI's whole answer to the TUI's config
// form, so a key the form can set and this cannot is the parity gap reopening. The expectation is
// read off the struct rather than listed, so a key added to Config tomorrow fails here.
func TestConfigKeysCoverEverySettableKey(t *testing.T) {
	have := map[string]bool{}
	for _, k := range configKeys {
		have[k.name] = true
	}
	ct := reflect.TypeOf(config.Config{})
	for i := 0; i < ct.NumField(); i++ {
		f := ct.Field(i)
		tag := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if tag == "" || tag == "-" { // not persisted (ArchitectureSet)
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct { // github:/lint: blocks are settable per leaf
			for j := 0; j < ft.NumField(); j++ {
				leaf := strings.Split(ft.Field(j).Tag.Get("yaml"), ",")[0]
				if leaf == "" || leaf == "-" {
					continue
				}
				if !have[tag+"."+leaf] {
					t.Errorf("config key %q is settable in the TUI form's file but not by `repo config`", tag+"."+leaf)
				}
			}
			continue
		}
		if !have[tag] {
			t.Errorf("config key %q is not settable by `repo config`", tag)
		}
	}
}

// TestSetOneKeyLeavesTheRestAlone is the guard on the failure that makes a config editor dangerous:
// a save rewrites the whole file, so setting one key must not disturb any other. `verify` is the
// one that hurts — losing it silently removes the repo's submit gate for the whole fleet.
func TestSetOneKeyLeavesTheRestAlone(t *testing.T) {
	on, maxLines := true, 400
	base := config.Config{
		Architecture: "ARCH.md", Containerfile: "Containerfile", ReviewPrompt: "prompt.md",
		Verify: "scripts/verify.sh", Reference: "trunk", Reading: []string{"ARCH.md"},
		Lint: api.Lint{MaxLines: &maxLines}, GitHub: api.GitHub{Issues: &on},
	}
	for _, k := range configKeys {
		cfg := base // the command edits a copy of the config as loaded
		if err := k.apply(&cfg, sampleValue(k.name)); err != nil {
			t.Fatalf("%s: %v", k.name, err)
		}
		for _, other := range configKeys {
			if other.name == k.name {
				continue
			}
			if got, want := other.show(cfg), other.show(base); got != want {
				t.Errorf("setting %s changed %s: %q → %q", k.name, other.name, want, got)
			}
		}
	}
}

// sampleValue is a valid new value per key — the point is that it differs from base.
func sampleValue(key string) string {
	switch key {
	case "github.issues":
		return "off"
	case "lint.max_lines":
		return "500"
	case "lint.max_comment_avg":
		return "3.5"
	case "reading":
		return "README.md,ARCH.md"
	default:
		return "somewhere/else.md"
	}
}

// TestUnsetIsNotZero: clearing a key must restore the default, not pin a deliberate zero. An empty
// max_lines means "the built-in bound", and a bound of 0 would fail every file in the repo.
func TestUnsetIsNotZero(t *testing.T) {
	n, on := 400, true
	cfg := config.Config{Lint: api.Lint{MaxLines: &n}, GitHub: api.GitHub{Issues: &on}}
	for _, name := range []string{"lint.max_lines", "lint.max_comment_avg", "github.issues"} {
		k, err := findConfigKey(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := k.apply(&cfg, ""); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if cfg.Lint.MaxLines != nil {
		t.Errorf("clearing lint.max_lines pinned %d instead of unsetting it", *cfg.Lint.MaxLines)
	}
	if cfg.GitHub.Issues != nil {
		t.Errorf("clearing github.issues pinned %v — unset means on (the default), which is not the same as off", *cfg.GitHub.Issues)
	}
	if got := mustKey(t, "github.issues").show(cfg); !strings.Contains(got, "default") {
		t.Errorf("an unset toggle should read as the default, got %q", got)
	}
}

// TestBadValuesAreRefusedByName: a wrong value must be refused here, naming the key, rather than
// travelling to the hub to come back as a message about YAML.
func TestBadValuesAreRefusedByName(t *testing.T) {
	for _, tc := range []struct{ key, val string }{
		{"github.issues", "maybe"},
		{"lint.max_lines", "lots"},
		{"lint.max_comment_avg", "3.5x"},
	} {
		var cfg config.Config
		err := mustKey(t, tc.key).apply(&cfg, tc.val)
		if err == nil {
			t.Errorf("%s accepted %q", tc.key, tc.val)
			continue
		}
		if !strings.Contains(err.Error(), tc.key) {
			t.Errorf("%s's error does not name the key: %v", tc.key, err)
		}
	}
	// And an unknown key must list the real ones — the user needs the name they meant.
	_, err := findConfigKey("verfy")
	if err == nil || !strings.Contains(err.Error(), "verify") {
		t.Errorf("an unknown key should fail loud and list the known keys, got %v", err)
	}
}

func mustKey(t *testing.T, name string) configKey {
	t.Helper()
	k, err := findConfigKey(name)
	if err != nil {
		t.Fatal(err)
	}
	return k
}
