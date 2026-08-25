// package: ui/cli / repo config
// type:    command (host CLI)
// job:     `repo config [key [value]]` — read and set the current repo's
// .sindri/config.yaml keys, the CLI half of the TUI's repo-config form.
// A set reads the config, changes one key, and writes it back, so the
// keys it does not name survive; the hub validates before anything lands.
// limits:  key parsing and printing; validation is the config package's, hub-side.
package cli

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/flo-at/sindri/internal/config"
	"github.com/spf13/cobra"
)

// configKey is one settable key: how to read it for display and how to apply a new value.
// Keeping both halves together is what stops `repo config` from listing a key it cannot set.
type configKey struct {
	name  string
	help  string
	show  func(c config.Config) string
	apply func(c *config.Config, v string) error
}

// configKeys is the settable surface, in display order — verify first, since it is the only one a
// repo cannot work without. Deliberately the whole file, not just the keys the TUI form shows: a
// CLI that could only reach some of them would just be a smaller gap in the same wall.
var configKeys = []configKey{
	{"verify", "REQUIRED — repo-relative script the submit gate runs; without it nothing can be submitted",
		func(c config.Config) string { return c.Verify },
		func(c *config.Config, v string) error { c.Verify = v; return nil }},
	{"architecture", "repo-relative architecture doc (default ARCHITECTURE.md)",
		func(c config.Config) string { return c.Architecture },
		func(c *config.Config, v string) error { c.Architecture = v; return nil }},
	{"containerfile", "repo-relative image recipe (empty = filename discovery)",
		func(c config.Config) string { return c.Containerfile },
		func(c *config.Config, v string) error { c.Containerfile = v; return nil }},
	{"review_prompt", "repo-relative reviewer-prompt file (empty = built-in prompt)",
		func(c config.Config) string { return c.ReviewPrompt },
		func(c *config.Config, v string) error { c.ReviewPrompt = v; return nil }},
	{"reference", "branch agents work from and merge into (empty = the checkout's current branch)",
		func(c config.Config) string { return c.Reference },
		func(c *config.Config, v string) error { c.Reference = v; return nil }},
	{"reading", "comma-separated docs a planner must read first",
		func(c config.Config) string { return strings.Join(c.Reading, ",") },
		func(c *config.Config, v string) error { c.Reading = splitList(v); return nil }},
	{"github.issues", "on|off — the GitHub issue source (default on)",
		func(c config.Config) string { return onOff(c.GitHub.Issues) },
		func(c *config.Config, v string) error {
			if v == "" { // unset, so the default (on) applies again — not the same as "off"
				c.GitHub.Issues = nil
				return nil
			}
			b, err := parseOnOff(v)
			if err != nil {
				return err
			}
			c.GitHub.Issues = &b
			return nil
		}},
	{"lint.max_lines", "integer cap on a source file's length (empty = the default)",
		func(c config.Config) string { return intStr(c.Lint.MaxLines) },
		func(c *config.Config, v string) error {
			n, err := parseIntPtr(v, "lint.max_lines")
			c.Lint.MaxLines = n
			return err
		}},
	{"lint.max_comment_avg", "mean lines per comment block (empty = the default)",
		func(c config.Config) string { return floatStr(c.Lint.MaxCommentAvg) },
		func(c *config.Config, v string) error {
			f, err := parseFloatPtr(v, "lint.max_comment_avg")
			c.Lint.MaxCommentAvg = f
			return err
		}},
}

// splitList parses a comma-separated list, dropping blanks so "a,,b" and "a, b" both mean two docs.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// onOff renders a tri-state toggle: unset is not "off", it is "on (default)".
func onOff(b *bool) string {
	switch {
	case b == nil:
		return "on (default)"
	case *b:
		return "on"
	default:
		return "off"
	}
}

func parseOnOff(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "on", "true", "yes":
		return true, nil
	case "off", "false", "no":
		return false, nil
	}
	return false, fmt.Errorf("github.issues takes on or off, got %q", v)
}

func intStr(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

func floatStr(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'g', -1, 64)
}

// parseIntPtr maps "" to unset (nil) rather than to 0 — a bound of zero is not "no bound".
func parseIntPtr(v, key string) (*int, error) {
	if v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil, fmt.Errorf("%s takes a whole number, got %q", key, v)
	}
	return &n, nil
}

func parseFloatPtr(v, key string) (*float64, error) {
	if v == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil, fmt.Errorf("%s takes a number, got %q", key, v)
	}
	return &f, nil
}

// findConfigKey resolves a key name, failing loud with the full list. An unknown key must not
// reach the hub: it would either be ignored or rejected with a message about YAML, when what the
// user needs is the name they meant.
func findConfigKey(name string) (configKey, error) {
	for _, k := range configKeys {
		if k.name == name {
			return k, nil
		}
	}
	names := make([]string, 0, len(configKeys))
	for _, k := range configKeys {
		names = append(names, k.name)
	}
	sort.Strings(names)
	return configKey{}, fmt.Errorf("unknown config key %q — known keys: %s", name, strings.Join(names, ", "))
}

// repoConfigCmd is the CLI half of the TUI's repo-config form: same keys, same validation.
func repoConfigCmd() *cobra.Command {
	var long strings.Builder
	long.WriteString("Read or set the CURRENT repo's .sindri/config.yaml (the repo you are standing in).\n\n" +
		"  sindri repo config                  show every key and its resolved value\n" +
		"  sindri repo config <key>            show one key\n" +
		"  sindri repo config <key> <value>    set it (empty value unsets, restoring the default)\n\n" +
		"The hub validates before writing, so a path that escapes the repo or names a missing\n" +
		"file is refused rather than persisted. Keys:\n")
	for _, k := range configKeys {
		fmt.Fprintf(&long, "  %-20s %s\n", k.name, k.help)
	}
	return &cobra.Command{
		Use: "config [key] [value]", Short: "Show or set a key in the current repo's .sindri/config.yaml",
		Long: long.String(), Args: cobra.MaximumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withHub(func(b backend) error {
				d, err := b.RepoInfo("") // "" = the repo this invocation is standing in
				if err != nil {
					return err
				}
				if len(args) == 0 {
					for _, k := range configKeys {
						fmt.Printf("%-20s %s\n", k.name, dash(k.show(d.Config)))
					}
					return nil
				}
				key, err := findConfigKey(args[0])
				if err != nil {
					return err
				}
				if len(args) == 1 {
					fmt.Println(key.show(d.Config))
					return nil
				}
				// Change one key on the config as loaded: a save rewrites the whole file, so
				// building a fresh struct here would delete every key not named on this line.
				cfg := d.Config
				if err := key.apply(&cfg, args[1]); err != nil {
					return err
				}
				if err := b.WriteRepoConfig(cfg); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "set %s = %s\n", key.name, dash(key.show(cfg)))
				return nil
			})
		},
	}
}
