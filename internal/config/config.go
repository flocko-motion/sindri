// package: config / config
// type:    logic (per-project config loader + validator)
// job:     read and validate a repo's .sindri/config.yaml (overlaid on an optional
// global config + defaults): the one declarative place a project sets its
// architecture-doc path, image recipe, reviewer prompt, and GitHub toggle.
// Fail-loud — any invalid config is an error; an absent file keeps defaults.
// limits:  pure loader/validator, no hub/adapter/UI deps — callers wire the values.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/tools/paths"
	"gopkg.in/yaml.v3"
)

// defaultArchitecture is looked for when `architecture` is unset, but never created.
const defaultArchitecture = "ARCHITECTURE.md"

// GitHub is the `github:` block. It crosses the wire in its own right (Config does),
// so it is internal/api.GitHub under the name every existing caller here already uses.
type GitHub = api.GitHub

// Lint is the `lint:` block; it crosses the wire, so it is internal/api.Lint under
// the name every existing caller here already uses.
type Lint = api.Lint

// Config is a project's resolved .sindri/config.yaml (repo over global over
// default). It crosses the wire (the TUI's repo-config editor reads and writes it),
// so it is internal/api.Config under the name every existing caller here already
// uses; Load/Write/validate/Abs stay here since they touch disk.
type Config = api.Config

// Load layers repo config over global over defaults. Absent is fine; malformed is an error.
func Load(root string) (Config, error) {
	var c Config
	if err := decodeInto(filepath.Join(paths.StateDir(), "config.yaml"), &c); err != nil {
		return Config{}, err
	}
	if err := decodeInto(filepath.Join(root, ".sindri", "config.yaml"), &c); err != nil {
		return Config{}, err
	}
	c.ArchitectureSet = c.Architecture != ""
	if c.Architecture == "" {
		c.Architecture = defaultArchitecture
	}
	if err := validate(c, root); err != nil {
		return Config{}, err
	}
	return c, nil
}

// decodeInto overlays path onto c (absent = no-op); KnownFields makes a stray key fail loud.
func decodeInto(path string, c *Config) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil && !errors.Is(err, io.EOF) { // EOF = empty file, fine
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// validate rejects escaping paths and missing set files; the default architecture is exempt
// because the hub only recommends one (Hub.StartupAdvice).
func validate(c Config, root string) error {
	checks := []struct {
		key, val  string
		mustExist bool
	}{
		{"architecture", c.Architecture, c.ArchitectureSet},
		{"containerfile", c.Containerfile, c.Containerfile != ""},
		{"review_prompt", c.ReviewPrompt, c.ReviewPrompt != ""},
	}
	for _, ch := range checks {
		if ch.val == "" {
			continue
		}
		abs, err := repoRel(root, ch.val)
		if err != nil {
			return fmt.Errorf(".sindri/config.yaml: %s %q must be a repo-relative path inside the project (%v)", ch.key, ch.val, err)
		}
		if ch.mustExist {
			if _, err := os.Stat(abs); err != nil {
				return fmt.Errorf(".sindri/config.yaml: %s %q — file not found at %s", ch.key, ch.val, abs)
			}
		}
	}
	return nil
}

// Write persists c, validating first so a broken config never lands; unset keys stay omitted.
func Write(root string, c Config) error {
	c.ArchitectureSet = c.Architecture != "" && c.Architecture != defaultArchitecture
	if err := validate(c, root); err != nil {
		return err
	}
	out := map[string]any{}
	if c.Architecture != "" && c.Architecture != defaultArchitecture {
		out["architecture"] = c.Architecture
	}
	if c.Containerfile != "" {
		out["containerfile"] = c.Containerfile
	}
	if c.ReviewPrompt != "" {
		out["review_prompt"] = c.ReviewPrompt
	}
	if c.GitHub.Issues != nil {
		out["github"] = map[string]any{"issues": *c.GitHub.Issues}
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	dir := filepath.Join(root, ".sindri")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Abs resolves a repo-relative config path against root ("" stays "").
func Abs(root, rel string) string {
	if rel == "" {
		return ""
	}
	return filepath.Join(root, rel)
}

// repoRel cleans rel against root; absolute paths and any escaping ".." are errors.
func repoRel(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute path")
	}
	abs := filepath.Clean(filepath.Join(root, rel))
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", errors.New("escapes the project root")
	}
	return abs, nil
}
