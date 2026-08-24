// package: tui / editor
// type:    ui (handing a tree to another program)
// job:     open a workspace in the user's editor or a shell — which editor would run,
// where it is rooted, and the materialization a PR needs before there is a
// tree to open at all.
// limits:  builds the command and reports what it would be; running it is the loop's
// (-> tui.go, which execs it and repaints afterwards).
package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// shellAt builds an interactive shell rooted at dir (for opening a workspace).
func shellAt(dir string) *exec.Cmd {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "bash"
	}
	c := exec.Command(sh)
	c.Dir = dir
	return marked(c)
}

// editorAtCmd opens the editor on an agent's live workspace; no materialization, it already exists.
func (m *model) editorAtCmd(dir string) tea.Cmd {
	m.flash = "opening " + dir + " in " + editorName() + "…"
	return func() tea.Msg { return editorReadyMsg(dir) }
}

// openEditorCmd materializes a PR (the same checkout `verify` shells into) and opens the editor.
func (m *model) openEditorCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "opening " + id + " in " + editorName() + "…"
	return func() tea.Msg {
		path, err := cl.MaterializeReview(id)
		if err != nil {
			return errModalMsg{err}
		}
		return editorReadyMsg(path)
	}
}

// editorCandidates lists editors in the order a Unix tool looks: the user's choice, the
// distribution default, then vi, which POSIX requires, so the list can never come up empty.
func editorCandidates() []string {
	var out []string
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			out = append(out, v)
		}
	}
	return append(out, "sensible-editor", "editor", "vi")
}

// editorName is the editor that would run, for a flash message.
func editorName() string {
	for _, cand := range editorCandidates() {
		if bin, _, ok := resolveEditor(cand); ok {
			return filepath.Base(bin)
		}
	}
	return "an editor"
}

// resolveEditor splits a candidate ($EDITOR is often "code --wait") and checks PATH for the binary.
func resolveEditor(cand string) (bin string, args []string, ok bool) {
	fields := strings.Fields(cand)
	if len(fields) == 0 {
		return "", nil, false
	}
	p, err := exec.LookPath(fields[0])
	if err != nil {
		return "", nil, false
	}
	return p, fields[1:], true
}

// editorAt passes dir as the argument, so a file-browser editor (vim, emacs) lands on the tree
// rather than an empty buffer. nil when nothing is installed, for the caller to report.
func editorAt(dir string) *exec.Cmd {
	for _, cand := range editorCandidates() {
		bin, args, ok := resolveEditor(cand)
		if !ok {
			continue
		}
		c := exec.Command(bin, append(args, ".")...)
		c.Dir = dir
		return marked(c) // an editor with a built-in terminal is the same door as a shell
	}
	return nil
}

// verifyCmd materializes a PR for review, then signals the loop to open a shell there.
func (m *model) verifyCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "materializing " + id + " for review…"
	return func() tea.Msg {
		path, err := cl.MaterializeReview(id)
		if err != nil {
			return errModalMsg{err}
		}
		return reviewReadyMsg(path)
	}
}
