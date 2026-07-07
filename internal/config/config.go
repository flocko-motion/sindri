// package: config / config
// type:    logic (per-project config loader + validator)
// job:     read and validate a repo's .sindri/config.yaml (overlaid on an optional
//          global config + defaults): the one declarative place a project sets its
//          architecture-doc path, image recipe, reviewer prompt, and GitHub toggle.
//          Fail-loud — any invalid config is an error; an absent file keeps defaults.
// limits:  pure loader/validator, no hub/adapter/UI deps — callers wire the values.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/paths"
	"gopkg.in/yaml.v3"
)

// defaultArchitecture is the architecture doc the reviewer is pointed at when no
// `architecture` key is set (and the one the hub seeds).
const defaultArchitecture = "ARCHITECTURE.md"

// GitHub is the `github:` block.
type GitHub struct {
	// Issues toggles the GitHub issue source. nil (key unset) means the default —
	// ON (opt-out): a repo imports its open issues unless it sets `issues: false`.
	Issues *bool `yaml:"issues"`
}

// Config is a project's resolved .sindri/config.yaml (repo over global over default).
type Config struct {
	Architecture  string `yaml:"architecture"`  // repo-relative architecture doc (default ARCHITECTURE.md)
	Containerfile string `yaml:"containerfile"` // repo-relative image recipe ("" = filename discovery)
	ReviewPrompt  string `yaml:"review_prompt"` // repo-relative reviewer-prompt file ("" = default prompt)
	GitHub        GitHub `yaml:"github"`

	// ArchitectureSet is true when `architecture` was explicitly configured (at either
	// layer), so the hub seeds the placeholder ONLY when it's unset.
	ArchitectureSet bool `yaml:"-"`
}

// Load reads and validates a project's config: the global config under the hub's
// state dir first (the base), then the repo's .sindri/config.yaml overlaid on top,
// then defaults. Absent files are not an error; a malformed file, an unknown key, a
// wrong-typed value, or a path that is absolute / escapes the repo / (when set) names
// a missing file all ARE — surfaced with the file and the problem, never defaulted.
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
	if err := c.validate(root); err != nil {
		return Config{}, err
	}
	return c, nil
}

// decodeInto overlays the config file at path onto c if it exists (absent = no-op).
// KnownFields(true) makes an unrecognized key a decode error — fail-loud for free.
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

// validate rejects path keys that are absolute or escape the repo, and (when a key is
// set) a target file that doesn't exist. The default architecture (key unset) is
// exempt: a missing default is seeded by the hub, not an error.
func (c Config) validate(root string) error {
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

// IssuesEnabled reports whether the GitHub issue source is on. It defaults to ON
// (opt-out): a repo imports its open issues unless it explicitly sets
// `github.issues: false`. The source still degrades to absent whenever gh is
// missing / unauthenticated / offline or the repo has no GitHub remote.
func (c Config) IssuesEnabled() bool {
	return c.GitHub.Issues == nil || *c.GitHub.Issues
}

// Write serializes c to <root>/.sindri/config.yaml, first validating it against
// root so a broken config is never persisted (the caller surfaces the error). Only
// keys the caller set are written — empty paths and an unset github toggle are
// omitted — so the file stays clean and unset keys keep defaulting.
func Write(root string, c Config) error {
	c.ArchitectureSet = c.Architecture != "" && c.Architecture != defaultArchitecture
	if err := c.validate(root); err != nil {
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

// repoRel validates that rel is a repo-relative path resolving inside root, returning
// the cleaned absolute path. Absolute paths and any ".." that escapes root are errors.
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
