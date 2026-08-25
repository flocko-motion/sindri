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

// ErrConfig marks a failure to read a project's configuration, so callers can tell it apart from a
// fault that might pass on its own. It never will: a human edits the file or nothing changes, and
// anything told to "try again later" instead waits forever.
var ErrConfig = errors.New("project configuration")

// Load layers repo config over global over defaults. Absent is fine; malformed is an error.
func Load(root string) (Config, error) {
	c, err := load(root)
	if err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrConfig, err)
	}
	return c, nil
}

func load(root string) (Config, error) {
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
		return fmt.Errorf("%s: %w%s", path, err, staleBinaryHint(err))
	}
	return nil
}

// staleBinaryHint names the likeliest cause of an unknown key, which is not a typo: a config that
// gained a setting a LONGER-RUNNING process does not know yet. The hub holds a config open for its
// whole life, so a key added by an upgrade or a merge reads as invalid until it restarts — and
// without saying so, a strict parse looks like a broken file nobody edited.
func staleBinaryHint(err error) string {
	if !strings.Contains(err.Error(), "not found in type") {
		return ""
	}
	return "\nIf that key was added recently, the process reading this config predates it — restart it" +
		" (`sindri hub stop`, then `sindri hub start --bg`) so it knows the setting."
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
		// verify is NOT here: it is a shell command, not a path (-> api.Config.Verify). Validating it
		// as one rejected `make check`, and the rule bought no safety anyway — a script it did accept
		// is arbitrary code the moment it runs.
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

// lintOut emits only the lint keys that are actually set. The pointers are the point: an unset
// bound must stay absent so the default still applies, rather than being written out as a zero.
func lintOut(l Lint) map[string]any {
	out := map[string]any{}
	if l.MaxLines != nil {
		out["max_lines"] = *l.MaxLines
	}
	if l.MaxCommentAvg != nil {
		out["max_comment_avg"] = *l.MaxCommentAvg
	}
	return out
}

// Write persists c, validating first so a broken config never lands; unset keys stay omitted.
// It rewrites the whole file from c, so c must be a config that was LOADED and then modified —
// handing it a freshly built struct silently drops every key that struct left unset.
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
	if c.Verify != "" {
		out["verify"] = c.Verify
	}
	if c.GitHub.Issues != nil {
		out["github"] = map[string]any{"issues": *c.GitHub.Issues}
	}
	if c.Reference != "" {
		out["reference"] = c.Reference
	}
	if len(c.Reading) > 0 {
		out["reading"] = c.Reading
	}
	if lint := lintOut(c.Lint); len(lint) > 0 {
		out["lint"] = lint
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
