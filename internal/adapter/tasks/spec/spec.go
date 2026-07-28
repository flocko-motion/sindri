// package: spec
// type:    adapter (external tool)
// job:     wraps the openspec CLI for the lint gate — detect whether a project
//          uses openspec and validate its specs via `openspec validate`.
// limits:  read-only; the propose/apply/archive workflow runs via the openspec
//          CLI in agent containers, not here.
package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/hub/task"
)

// ID derives a stable os-XXXXXX task id from an openspec change name (the id scheme
// that namespaces the openspec source; the one-way hash means the hub reverses it by
// matching over the current change names).
func ID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "os-" + hex.EncodeToString(sum[:])[:6]
}

// Source adapts openspec as a task source: each active change becomes a task.
type Source struct{}

// Enabled reports whether the repo uses openspec.
func (Source) Enabled(root string) bool { return Enabled(root) }

// Tasks maps the repo's active openspec changes to domain tasks (os-* ids, a
// progress-annotated title, closed when all the change's tasks are done). Local
// read — force is moot.
func (Source) Tasks(root string, _ bool) ([]task.Task, error) {
	changes, err := Changes(root)
	if err != nil {
		return nil, err
	}
	out := make([]task.Task, 0, len(changes))
	for _, c := range changes {
		status := "open"
		if c.Done() {
			status = "closed"
		}
		out = append(out, task.Task{
			ID:          ID(c.Name),
			Title:       fmt.Sprintf("%s (%d/%d)", c.Name, c.CompletedTasks, c.TotalTasks),
			Status:      status,
			Type:        "spec",
			Description: Proposal(root, c.Name),
		})
	}
	return out, nil
}

// Proposal is a change's proposal.md — its description. An openspec change is a prose
// document, and the proposal is the part that says what the change is for, so it is what a
// board or detail view should show. (`openspec list --json` carries only a name and task
// counts, hence reading the file.)
//
// The leading `# Heading` is skipped: the title column already carries the change's name.
// Best-effort — design.md and tasks.md can stand alone, so a change with no proposal yields
// an empty description and the task listing still succeeds.
func Proposal(projectRoot, name string) string {
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return "" // never let a change name walk out of the changes directory
	}
	b, err := os.ReadFile(filepath.Join(projectRoot, "openspec", "changes", name, "proposal.md"))
	if err != nil {
		return ""
	}
	body := strings.TrimSpace(string(b))
	if rest, ok := strings.CutPrefix(body, "#"); ok {
		if _, after, found := strings.Cut(rest, "\n"); found {
			body = strings.TrimSpace(after)
		}
	}
	return body
}

// OnMerged is a no-op for openspec: a change is archived at close/scrap time (see
// Finish/Archive), not as a side effect of a PR merging.
func (Source) OnMerged(root, taskID, note string) error { return nil }

// Finish archives (done) or removes (scrap) the openspec change behind an os- id.
// handled is false for a non-os id; an os id whose change can't be resolved is a
// real error (the id is a one-way hash, so a stale cache can't be reversed).
func (Source) Finish(root, taskID string, scrap bool) (bool, error) {
	if !strings.HasPrefix(taskID, "os-") {
		return false, nil
	}
	name, ok := changeName(root, taskID)
	if !ok {
		return true, fmt.Errorf("%s: can't resolve its openspec change (re-sync and retry)", taskID)
	}
	if scrap {
		return true, DeleteChange(root, name)
	}
	return true, Archive(root, name)
}

// changeName resolves an os-<hash> id back to its change name by matching ID over the
// current changes (the id is a one-way hash of the name).
func changeName(root, id string) (string, bool) {
	changes, err := Changes(root)
	if err != nil {
		return "", false
	}
	for _, c := range changes {
		if ID(c.Name) == id {
			return c.Name, true
		}
	}
	return "", false
}

// Change is an openspec change from `openspec list --json`.
type Change struct {
	Name           string `json:"name"`
	CompletedTasks int    `json:"completedTasks"`
	TotalTasks     int    `json:"totalTasks"`
	Status         string `json:"status"`
}

// Done reports whether a change's tasks are all complete.
func (c Change) Done() bool { return c.TotalTasks > 0 && c.CompletedTasks == c.TotalTasks }

// Changes lists the project's active openspec changes. Returns (nil, nil) when
// openspec isn't used (an optional source), but a CLI failure or unparseable output
// is returned as an error — after Enabled() is true, those are real failures, not a
// legitimate "no changes".
func Changes(projectRoot string) ([]Change, error) {
	if !Enabled(projectRoot) {
		return nil, nil
	}
	cmd := exec.Command("openspec", "list", "--json")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("openspec list in %s: %w", projectRoot, err)
	}
	var wrap struct {
		Changes []Change `json:"changes"`
	}
	if e := json.Unmarshal(out, &wrap); e != nil {
		return nil, fmt.Errorf("parse openspec list output: %w", e)
	}
	return wrap.Changes, nil
}

// Archive marks a change done: `openspec archive <name> --yes` moves it out of the
// active set and folds its deltas into the main specs. This is the "done" close for
// an openspec item. A CLI failure is surfaced.
func Archive(projectRoot, name string) error {
	cmd := exec.Command("openspec", "archive", name, "--yes")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("openspec archive %s: %s", name, lastLine(string(out)))
	}
	return nil
}

// DeleteChange scraps a change: it removes the change's proposal directory
// (openspec/changes/<name>) without touching the main specs — the "scrap" close.
// The dir is git-tracked, so a mistaken scrap is recoverable with git. The name is
// validated to a single path segment so it can't escape the changes dir.
func DeleteChange(projectRoot, name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return fmt.Errorf("invalid change name %q", name)
	}
	dir := filepath.Join(projectRoot, "openspec", "changes", name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("no such change %q at %s", name, dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("scrap change %s: %w", name, err)
	}
	return nil
}

// lastLine returns the last non-empty line of s (an openspec error is usually its
// final line), so a failure surfaces the reason, not the whole output.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return strings.TrimSpace(s)
}

// Enabled reports whether the project uses openspec (has an openspec/ dir).
func Enabled(projectRoot string) bool {
	info, err := os.Stat(filepath.Join(projectRoot, "openspec"))
	return err == nil && info.IsDir()
}

// CLIInstalled reports whether the openspec CLI is available on PATH.
func CLIInstalled() bool {
	_, err := exec.LookPath("openspec")
	return err == nil
}

// ValidatorName names the check Validate performs, for any surface that reports its
// verdict. `brokkr lint openspec` and `sindri openspec submit` both call Validate, so
// naming it tells an agent that one green light stands for both gates.
const ValidatorName = "openspec validate --all (the same check `brokkr lint openspec` and `sindri openspec submit` both run)"

// report is openspec's `--json` shape, reduced to the fields worth surfacing.
type report struct {
	Items []struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Valid  bool   `json:"valid"`
		Issues []struct {
			Level   string `json:"level"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"issues"`
	} `json:"items"`
}

// Validate runs `openspec validate --all --json`. A non-zero exit is a validation failure
// (ok=false); everything else is a non-failing skip (ok=true). openspec is OPTIONAL: a
// project with no openspec/ skips silently, but one that uses openspec yet lacks the CLI
// degrades with a visible note (in output) rather than vanishing — so a skipped validation
// is never mistaken for a passed one.
//
// It asks for --json because that is the format carrying the REASONS: per item, each
// issue's file and the rule it broke (e.g. `ADDED "A thing" must contain SHALL or MUST`),
// which is what a caller needs to fix the spec.
func Validate(projectRoot string) (ok bool, output string) {
	if !Enabled(projectRoot) {
		return true, "" // project doesn't use openspec — nothing to validate
	}
	if _, err := exec.LookPath("openspec"); err != nil {
		return true, "openspec/ present but the openspec CLI is not installed — skipping spec validation (optional)"
	}
	cmd := exec.Command("openspec", "validate", "--all", "--json")
	cmd.Dir = projectRoot
	out, err := cmd.Output() // stdout only: stderr noise must not corrupt the JSON
	failed := false
	if err != nil {
		if _, isExit := err.(*exec.ExitError); !isExit {
			return true, "openspec validate could not run: " + err.Error() // degrade, but visibly
		}
		failed = true // a real validation failure; stdout still carries the report
	}
	return !failed, formatReport(out, failed)
}

// formatReport renders openspec's JSON report as the lines a human or an agent acts on:
// one line per item, and beneath a failing one every issue with its file and the rule it
// broke. Unparseable JSON falls back to the raw output rather than swallowing it — a
// verdict with no explanation is still better than no verdict.
func formatReport(raw []byte, failed bool) string {
	var r report
	if err := json.Unmarshal(raw, &r); err != nil || len(r.Items) == 0 {
		return string(raw)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", ValidatorName)
	pass := 0
	for _, it := range r.Items {
		if it.Valid {
			pass++
			continue // a passing item needs no line; the totals below account for it
		}
		fmt.Fprintf(&b, "✗ %s/%s\n", it.Type, it.ID)
		for _, is := range it.Issues {
			where := is.Path
			if where == "" {
				where = it.ID
			}
			fmt.Fprintf(&b, "    %s: %s (%s)\n", where, is.Message, strings.ToLower(is.Level))
		}
	}
	fmt.Fprintf(&b, "Totals: %d passed, %d failed (%d items)\n", pass, len(r.Items)-pass, len(r.Items))
	return b.String()
}
