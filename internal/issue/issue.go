// Package issue is the headless domain model for sindri work items.
//
// An Issue wraps a td task (and, when linked, an openspec change). All label
// and state logic lives here — review gates, spec links, status classification
// — with no dependency on any UI (CLI or TUI). Interfaces consume Issue and
// render it; they do not reimplement this logic.
package issue

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// Review-gate labels are require-review-<type> / approved-review-<type>.
	// We strip only "require-"/"approved-", keeping "review-<type>" as the gate
	// name so it displays as "review <type>" (e.g. "review code").
	requirePrefix  = "require-"
	approvedPrefix = "approved-"
	requireMatch   = "require-review-"
	approvedMatch  = "approved-review-"
	specPrefix     = "spec:"
)

// Issue is a td task, optionally linked to an openspec change via a spec: label.
type Issue struct {
	ID        string
	Title     string
	Status    string
	Type      string
	Priority  string
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Gate is a single review gate and whether it has been approved.
type Gate struct {
	Name     string // e.g. "code", "security"
	Approved bool
}

// IsClosed reports whether the issue is in a terminal state.
func (i Issue) IsClosed() bool {
	switch i.Status {
	case "closed", "approved", "merged":
		return true
	}
	return false
}

// IsActive reports whether the issue is being worked on or reviewed.
func (i Issue) IsActive() bool {
	return i.Status == "in_progress" || i.Status == "in_review"
}

// IsOpen reports whether the issue is open (unclaimed).
func (i Issue) IsOpen() bool {
	return i.Status == "open"
}

// Spec returns the linked openspec change name (from a spec:<name> label), or "".
func (i Issue) Spec() string {
	for _, l := range i.Labels {
		if strings.HasPrefix(l, specPrefix) {
			return strings.TrimPrefix(l, specPrefix)
		}
	}
	return ""
}

// Gates returns the review gates declared on the issue, with approval state.
func (i Issue) Gates() []Gate {
	approved := map[string]bool{}
	var names []string
	for _, l := range i.Labels {
		if strings.HasPrefix(l, approvedMatch) {
			approved[strings.TrimPrefix(l, approvedPrefix)] = true
		}
	}
	for _, l := range i.Labels {
		if strings.HasPrefix(l, requireMatch) {
			names = append(names, strings.TrimPrefix(l, requirePrefix))
		}
	}
	gates := make([]Gate, 0, len(names))
	for _, n := range names {
		gates = append(gates, Gate{Name: n, Approved: approved[n]})
	}
	return gates
}

// MissingReviews returns the names of required reviews not yet approved.
func (i Issue) MissingReviews() []string {
	var missing []string
	for _, g := range i.Gates() {
		if !g.Approved {
			missing = append(missing, g.Name)
		}
	}
	return missing
}

var taskIDRe = regexp.MustCompile(`\(?(td-[0-9a-f]+)\)?`)

// TaskIDFromTitle extracts a td-xxxxxx task ID from a PR title, or "".
func TaskIDFromTitle(title string) string {
	if m := taskIDRe.FindStringSubmatch(title); len(m) > 1 {
		return m[1]
	}
	return ""
}

// LoadAll loads every issue from td (including closed) and orders them in
// three sections: open (td priority order), active (recent first), closed
// (recent first).
func LoadAll(projectRoot string) ([]Issue, error) {
	out, err := exec.Command("td", "-w", projectRoot, "list", "--json", "--limit", "100", "--all").Output()
	if err != nil {
		return nil, err
	}
	return parseAndSort(out)
}

// Load loads a single issue by ID via `td show <id> --json`.
func Load(projectRoot, id string) (Issue, error) {
	out, err := exec.Command("td", "-w", projectRoot, "show", id, "--json").Output()
	if err != nil {
		return Issue{}, err
	}
	var raw struct {
		ID        string   `json:"id"`
		Title     string   `json:"title"`
		Status    string   `json:"status"`
		Type      string   `json:"type"`
		Priority  string   `json:"priority"`
		Labels    []string `json:"labels"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Issue{}, err
	}
	created, _ := time.Parse(time.RFC3339Nano, raw.CreatedAt)
	updated, _ := time.Parse(time.RFC3339Nano, raw.UpdatedAt)
	return Issue{
		ID: raw.ID, Title: raw.Title, Status: raw.Status, Type: raw.Type,
		Priority: raw.Priority, Labels: raw.Labels, CreatedAt: created, UpdatedAt: updated,
	}, nil
}

func parseAndSort(jsonData []byte) ([]Issue, error) {
	var raw []struct {
		ID        string   `json:"id"`
		Title     string   `json:"title"`
		Status    string   `json:"status"`
		Type      string   `json:"type"`
		Priority  string   `json:"priority"`
		Labels    []string `json:"labels"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return nil, err
	}
	items := make([]Issue, len(raw))
	for i, r := range raw {
		created, _ := time.Parse(time.RFC3339Nano, r.CreatedAt)
		updated, _ := time.Parse(time.RFC3339Nano, r.UpdatedAt)
		items[i] = Issue{
			ID:        r.ID,
			Title:     r.Title,
			Status:    r.Status,
			Type:      r.Type,
			Priority:  r.Priority,
			Labels:    r.Labels,
			CreatedAt: created,
			UpdatedAt: updated,
		}
	}

	var open, active, closed []Issue
	for _, t := range items {
		switch {
		case t.IsActive():
			active = append(active, t)
		case t.IsClosed():
			closed = append(closed, t)
		default:
			open = append(open, t)
		}
	}
	byUpdatedDesc := func(s []Issue) func(i, j int) bool {
		return func(i, j int) bool { return s[i].UpdatedAt.After(s[j].UpdatedAt) }
	}
	sort.Slice(active, byUpdatedDesc(active))
	sort.Slice(closed, byUpdatedDesc(closed))

	result := make([]Issue, 0, len(items))
	result = append(result, open...)
	result = append(result, active...)
	result = append(result, closed...)
	return result, nil
}
