// package: hub / prlist
// type:    command (the `sindri prs` verb)
// job:     scope a PR listing to what the caller may usefully read — a worker its own,
// every other role the whole project — narrow it to the recent ones, cap it, and
// say what the cap left out.
// limits:  formatting and scoping only; the store holds the PRs and their approvals.
package hub

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
)

// prListUsage is prs's own help, printed alongside any refusal so the accepted form is never left
// to be guessed at from the error alone.
var prListUsage = "usage: prs [--limit N]\n" +
	"  Defaults to the 10 most recent active PRs (open, or changed within " + api.ActiveWindow.String() + "). A worker\n" +
	"  sees only its own; a planner, reviewer or coauthor sees the whole project's."

// parsePRListFlags reads prs's only flag. An unknown flag or value is REFUSED, not ignored —
// silently swallowing one leaves the next mistyped flag looking like it worked too.
func parsePRListFlags(args []string) (int, error) {
	limit := 10
	for i := 0; i < len(args); i++ {
		name, inline, hasInline := strings.Cut(args[i], "=")
		switch name {
		case "--limit":
			val := inline
			if !hasInline {
				if i+1 >= len(args) {
					return limit, fmt.Errorf("%s needs a value", name)
				}
				i++
				val = args[i]
			}
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 {
				return limit, fmt.Errorf("--limit wants a positive number, got %q", val)
			}
			limit = n
		default:
			return limit, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	return limit, nil
}

// prListSummary is the line prs closes with, so a capped or narrowed listing can never read as the
// whole of it — the same reason task list and mail list both close on one. open/closed are counted
// over everything the caller may read, not just the active ones shown, so a truncated view still
// states the full shape of what stands behind it. mine marks a worker's own-scoped listing: "in
// total" there would read as the project's whole shape rather than just this agent's, so it says
// "of yours" instead — the same silent-completeness risk the footer exists to close, on the other axis.
func prListSummary(shown, active int, visible []api.PR, mine bool) string {
	open, closed := 0, 0
	for _, p := range visible {
		if api.PROpen(p) {
			open++
		} else {
			closed++
		}
	}
	total := "in total"
	var head string
	switch {
	case mine && shown < active:
		total = "of yours"
		head = fmt.Sprintf("showing %d of your %d active %s (`--limit %d` to widen)", shown, active, plural(active, "PR"), active)
	case mine:
		total = "of yours"
		head = fmt.Sprintf("showing %d of your active %s", shown, plural(shown, "PR"))
	case shown < active:
		head = fmt.Sprintf("showing %d of %d active %s (`--limit %d` to widen)", shown, active, plural(active, "PR"), active)
	default:
		head = fmt.Sprintf("showing %d active %s", shown, plural(shown, "PR"))
	}
	return fmt.Sprintf("%s — %d open, %d closed %s", head, open, closed, total)
}

// plural is the noun for n of something, for a count a human reads.
func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// cmdListPRs lists the PRs the caller may usefully read: a worker's own, everyone else's whole
// project, narrowed to the active ones and capped — a worker with three PRs was reading a hundred
// lines about every other agent's to find them.
func (h *Hub) cmdListPRs(c registry.Caller, args []string, out io.Writer) (int, error) {
	limit, err := parsePRListFlags(args)
	if err != nil {
		fmt.Fprintf(out, "%v\n%s\n", err, prListUsage)
		return 2, nil
	}
	ps := h.store.For(c.Project)
	all, err := ps.PRs()
	if err != nil {
		return 1, err
	}
	if len(all) == 0 {
		fmt.Fprintln(out, "no PRs")
		return 0, nil
	}
	visible := all
	if c.Role == "worker" {
		visible = make([]api.PR, 0, len(all))
		for _, p := range all {
			if p.Agent == c.Agent {
				visible = append(visible, p)
			}
		}
	}
	// ps.PRs() returns newest first; FilterPRs keeps that order, so the cap below drops the
	// oldest rows rather than an arbitrary set.
	active := api.FilterPRs(api.PRFilterActive, visible)
	shown := active
	if len(shown) > limit {
		shown = shown[:limit]
	}
	if len(shown) > 0 {
		counts, err := ps.ApprovalCounts()
		if err != nil {
			return 1, err
		}
		for _, p := range shown {
			fmt.Fprintf(out, "%-14s %-14s %-10s %s\n", p.ID, api.StatusLabel(p.Status, counts[p.ID]), p.Agent, p.Branch)
		}
	}
	fmt.Fprintln(out, prListSummary(len(shown), len(active), visible, c.Role == "worker"))
	return 0, nil
}
