// package: spec
// type:    adapter (external tool)
// job:     wraps the openspec CLI for the lint gate — detect whether a project
// uses openspec and validate its specs via `openspec validate`.
// limits:  read-only; the propose/apply/archive workflow runs via the openspec
// CLI in agent containers, not here.
package spec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/hub/task"
)

// ID derives a stable os-XXXXXX task id from a change name. One-way — reversed by changeName.
func ID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return task.SpecID(hex.EncodeToString(sum[:])[:6])
}

// Source adapts openspec as a task source: each active change becomes a task.
type Source struct{}

// Name identifies this source for comment-thread storage.
func (Source) Name() string { return "openspec" }

// Enabled reports whether the repo uses openspec.
func (Source) Enabled(root string) bool { return Enabled(root) }

// ToolMissing: the repo has an openspec/ dir but the openspec CLI isn't on PATH.
func (Source) ToolMissing(root string) bool { return Enabled(root) && !CLIInstalled() }

// Validate is this source doubling as a quality gate (-> adapter/gate.Gate): openspec's own
// validation, run wherever a submit path needs one. Delegates to the package's Validate so every
// caller agrees on what "passing" means.
func (Source) Validate(wt string) (bool, string) { return Validate(wt) }

// Tasks maps active changes to os-* tasks, closed once all of a change's own tasks are done.
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
			UpdatedAt:   changedAt(root, c.Name),
		})
	}
	return out, nil
}

// changedAt is when a change last changed: the newest mtime under its directory, zero if it cannot
// be read. openspec keeps no timestamps, so the files are the whole record — ticking a box in
// tasks.md is what makes a change recent. Dated because an undated task can never be recent, which
// kept every finished change out of the "active" filter that exists to show work just completed.
// A fresh clone stamps every file at checkout, so shortly after one the changes all read as new.
func changedAt(projectRoot, name string) time.Time {
	if !safeChangeName(name) {
		return time.Time{}
	}
	var newest time.Time
	dir := filepath.Join(projectRoot, "openspec", "changes", name)
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry dates nothing; the rest still do
		}
		if info, e := d.Info(); e == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

// safeChangeName rejects a name that would walk out of the changes directory.
func safeChangeName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/\\") && name != "." && name != ".."
}

// Proposal is a change's proposal.md — what the change is FOR, so it's what a board or detail view
// shows (`openspec list --json` has only name and counts). Leading `# Heading` dropped; missing → "".
func Proposal(projectRoot, name string) string {
	if !safeChangeName(name) {
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

// OnMerged is a no-op: a change is archived at close/scrap time, not as a side effect of a merge.
func (Source) OnMerged(root, taskID, note string) error { return nil }

// Finish archives (done) or removes (scrap) the change behind an os- id; handled is false for a
// non-os id. An os id whose change can't be resolved is a real error (the id is a one-way hash).
func (Source) Finish(root, taskID string, scrap bool) (bool, error) {
	if task.OwnerOf(taskID) != task.OwnerOpenSpec {
		return false, nil
	}
	name, ok := changeName(root, taskID)
	if !ok {
		// Not among the ACTIVE changes. Ending it is what this call is for, so already ended is the
		// postcondition, not a failure — a worker that archived the change itself, as part of the
		// work, otherwise had its checkpoint fail on the hub and was escalated for finishing.
		// A scrap still says so: there the caller means to destroy something it expects to find.
		if scrap {
			return true, fmt.Errorf("%s: can't resolve its openspec change to scrap (re-sync and retry)", taskID)
		}
		return true, nil
	}
	if scrap {
		return true, DeleteChange(root, name)
	}
	return true, Archive(root, name)
}

// Comments: an openspec change keeps no thread of its own — ok is always false.
func (Source) Comments(root, taskID string) ([]task.Comment, bool, error) { return nil, false, nil }

// AddComment: same reason as Comments — handled is always false.
func (Source) AddComment(root, taskID, body string) (bool, error) { return false, nil }

// changeName reverses an os-<hash> id by matching ID over the current changes (the hash is one-way).
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

// Changes lists the project's active openspec changes; (nil, nil) when openspec isn't used. Once
// Enabled is true, a CLI or parse failure is a real error, not a legitimate "no changes".
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

// Archive is the "done" close: `openspec archive --yes` folds the change's deltas into the specs.
func Archive(projectRoot, name string) error {
	cmd := exec.Command("openspec", "archive", name, "--yes")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("openspec archive %s: %s", name, lastLine(string(out)))
	}
	return nil
}

// DeleteChange is the "scrap" close: it removes openspec/changes/<name>, leaving the main specs
// alone. The dir is git-tracked, so a mistaken scrap is recoverable. The name must be one segment.
func DeleteChange(projectRoot, name string) error {
	if !safeChangeName(name) {
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

// lastLine is the last non-empty line of s — where openspec puts the reason for a failure.
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

// Version reports the openspec CLI's own version (ctx-bounded so a wedged CLI can't hang a caller,
// e.g. hosttools.Versions checking this at hub startup). Only the first line: symmetric with the
// pod-side manifest, which only ever captures the one build-log line its marker echo produced.
func Version(ctx context.Context) (string, bool) {
	out, err := exec.CommandContext(ctx, "openspec", "--version").Output()
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(out))
	if i := strings.IndexByte(v, '\n'); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v, true
}

// ValidatorName names the check Validate performs. `brokkr lint openspec` and `sindri openspec
// submit` both call it, so naming it tells an agent one green light stands for both gates.
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

// Validate runs `openspec validate --all --json` (--json carries the REASONS: each issue's file and
// the rule it broke). Non-zero exit is a failure (ok=false), anything else a skip (ok=true) — and a
// project using openspec without the CLI says so, so a skip is never mistaken for a pass.
func Validate(projectRoot string) (ok bool, output string) {
	if !Enabled(projectRoot) {
		return true, "" // project doesn't use openspec — nothing to validate
	}
	if _, err := exec.LookPath("openspec"); err != nil {
		return true, "openspec/ present but the openspec CLI is not installed — skipping spec validation (optional)\n" +
			"    hint: install the openspec CLI and spec validation runs."
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

// formatReport renders the JSON report at the length its verdict deserves: a pass is one line, an
// empty project says so, and a failure lists each bad item with its file and rule. Unparseable JSON
// falls back to raw output rather than swallowing the verdict.
func formatReport(raw []byte, failed bool) string {
	var r report
	if err := json.Unmarshal(raw, &r); err != nil {
		return string(raw)
	}
	// A project with an openspec/ directory but nothing in it yet is a valid, passing, EMPTY report.
	// It used to take the unparseable branch above and dump the whole JSON blob.
	if len(r.Items) == 0 {
		return "openspec: no specs or changes to validate\n"
	}
	if !failed {
		// A pass needs its verdict, not a report: one line proving the delegated validator ran.
		return fmt.Sprintf("openspec: %d passed (openspec validate --all)\n", len(r.Items))
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
