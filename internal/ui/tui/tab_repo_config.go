// package: tui / repo config form
// type:    ui (repo configuration editor)
// job:     edit the active repo's .sindri/config.yaml through a form over its keys
// (verify, architecture, containerfile, review_prompt, github.issues) instead of
// hand-editing YAML — fetch the resolved config, prefill, save via the hub
// (which validates, so a broken config is surfaced, never persisted).
// limits:  form wiring only; the fields/frame are component_form/_field.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
)

// repoConfigMsg carries the fetched config for the active repo, to open the form.
type repoConfigMsg struct {
	d   api.RepoDetail
	err error
}

// repoConfigCmd fetches the current repo's resolved config so the edit form can be
// prefilled with what's actually in effect.
func (m *model) repoConfigCmd() tea.Cmd {
	cl := m.cl
	m.flash = "loading repo config…"
	return func() tea.Msg {
		if cl == nil {
			return nil
		}
		d, err := cl.RepoInfo("") // "" = the client's current repo
		return repoConfigMsg{d: d, err: err}
	}
}

// openRepoConfigForm opens a form over the repo's .sindri/config.yaml keys, prefilled
// from the resolved config. Saving writes through the hub, which validates first — a
// bad value (e.g. a path that escapes the repo) comes back as an error modal rather
// than persisting a broken config.
func (m *model) openRepoConfigForm(d api.RepoDetail) {
	// First, because it is the only REQUIRED key: without it nothing can be submitted from this repo
	// at all (-> repo.Gate), and a form that hid it left the one thing to fix out of the one screen
	// for fixing it.
	verifyF := newTextField("verify (required)", d.Config.Verify)
	archF := newTextField("architecture", d.Config.Architecture)
	cfF := newTextField("containerfile", d.Config.Containerfile)
	rpF := newTextField("review_prompt", d.Config.ReviewPrompt)
	issues := "off"
	if d.IssuesEnabled { // resolved bool on the summary
		issues = "on"
	}
	issuesF := newChoiceField("github.issues", []string{"on", "off"}, []string{"on", "off"}, issues)

	cl := m.cl
	m.form.open("config: "+d.Name, []field{verifyF, archF, cfF, rpF, issuesF}, nil, func() tea.Cmd {
		on := issuesF.value() == "on"
		// Start from the config as loaded and change only the edited keys: a save rewrites the
		// whole file, so a struct built fresh from these fields would delete every key the form
		// does not show — reference, reading, lint.
		cfg := d.Config
		cfg.Verify = verifyF.value()
		cfg.Architecture, cfg.Containerfile, cfg.ReviewPrompt = archF.value(), cfF.value(), rpF.value()
		cfg.GitHub.Issues = &on
		return func() tea.Msg {
			if cl == nil {
				return nil
			}
			if err := cl.WriteRepoConfig(cfg); err != nil {
				return errModalMsg{err} // hub validation failed — surface it, don't persist
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}
