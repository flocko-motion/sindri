package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/cli"
	"github.com/flo-at/sindri/internal/ui/tui"
)

// TestAgentColumnOrderMatchesBothFrontEnds pins the Agents row's own field order — repo, then
// name, then role, then status — in both front-ends at once, so a change to one side's cell order
// without the other fails here instead of drifting unnoticed (sd-aadbb4 exists because it already
// had). Lives here, rather than inside tui or cli, because a test that imported both from either
// side would cycle back through cli's own import of tui. Checked as relative offsets, not exact
// column positions, so it does not re-pin the widths too.
func TestAgentColumnOrderMatchesBothFrontEnds(t *testing.T) {
	a := api.AgentView{Repo: "reponame", Name: "agentname", Role: "workerrole", Status: "busystate"}

	tuiLine := ansi.Strip(tui.AgentRowText(a))
	cliLine := ansi.Strip(cli.AgentListLine(a))

	for _, line := range []string{tuiLine, cliLine} {
		repo := strings.Index(line, a.Repo)
		name := strings.Index(line, a.Name)
		role := strings.Index(line, a.Role)
		status := strings.Index(line, a.Status)
		if repo < 0 || name < 0 || role < 0 || status < 0 {
			t.Fatalf("field missing from line %q (repo=%d name=%d role=%d status=%d)", line, repo, name, role, status)
		}
		if !(repo < name && name < role && role < status) {
			t.Errorf("want repo < name < role < status, got repo=%d name=%d role=%d status=%d in %q",
				repo, name, role, status, line)
		}
	}
}
