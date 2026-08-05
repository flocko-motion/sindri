// package: api / config
// type:    data (wire types + a pure predicate)
// job:     a project's resolved .sindri/config.yaml as it crosses the wire (the TUI's
// repo-config editor reads and writes it) and the blocks nested inside it.
// limits:  data and a pure predicate only; loading, writing and validating a config
// against disk stay in internal/config, which imports these.
package api

// GitHub is the `github:` block.
type GitHub struct {
	// Issues toggles the GitHub issue source; nil (unset) means ON — opt-out.
	Issues *bool `yaml:"issues"`
}

// Lint is the `lint:` block. Pointers: an unset key must differ from a deliberate zero.
type Lint struct {
	// MaxLines bounds a source file's length.
	MaxLines *int `yaml:"max_lines"`

	// MaxCommentAvg bounds the MEAN lines per comment block — a trend, not a per-comment cap.
	MaxCommentAvg *float64 `yaml:"max_comment_avg"`
}

// Config is a project's resolved .sindri/config.yaml (repo over global over default).
type Config struct {
	Architecture  string `yaml:"architecture"`  // repo-relative architecture doc (default ARCHITECTURE.md)
	Containerfile string `yaml:"containerfile"` // repo-relative image recipe ("" = filename discovery)
	ReviewPrompt  string `yaml:"review_prompt"` // repo-relative reviewer-prompt file ("" = default prompt)
	GitHub        GitHub `yaml:"github"`
	Lint          Lint   `yaml:"lint"`

	// Reference pins the branch agents branch from and merge into. Unset reads the main
	// checkout's current branch, so switching branches redefines it for the whole fleet.
	Reference string `yaml:"reference"`

	// Reading names the documents a planner must read first; it cannot guess them.
	Reading []string `yaml:"reading"`

	// ArchitectureSet marks an explicitly configured doc: only then must it exist (validate).
	ArchitectureSet bool `yaml:"-"`
}

// IssuesEnabled defaults to ON; the source still degrades to absent without gh or a remote.
func (c Config) IssuesEnabled() bool {
	return c.GitHub.Issues == nil || *c.GitHub.Issues
}
