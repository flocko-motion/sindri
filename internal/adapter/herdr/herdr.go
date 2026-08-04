// package: adapter/herdr / herdr
// type:    adapter (external tool: herdr)
// job:     tell herdr which agent occupies the current pane, and drop that claim on
// detach — argv built pure (like adapter/tmux) so the flags each herdr
// subcommand requires are covered by tests rather than discovered in use.
// limits:  builds and runs; WHEN to report is ui/attach's. Best-effort by design.
package herdr

import (
	"os"
	"os/exec"
)

// source identifies sindri's claims to herdr; release must name the same source that reported.
const source = "sindri"

// InPane reports whether we run inside a herdr-managed pane. Both vars are set on every pane, and
// the id is what every command targets.
func InPane() bool {
	return os.Getenv("HERDR_ENV") == "1" && os.Getenv("HERDR_PANE_ID") != ""
}

// ReportArgv claims the pane for name. report-metadata pins the display name too, or herdr's own
// detection labels the pane "claude" — the agent field alone drives the sidebar and toasts.
func ReportArgv(pane, name, state string) [][]string {
	return [][]string{
		reportAgentArgv(pane, name, state),
		{"pane", "report-metadata", pane, "--source", source, "--agent", name, "--display-agent", name},
	}
}

// StateArgv refreshes the live state during a long attach; ReportArgv already pinned the display.
func StateArgv(pane, name, state string) [][]string {
	return [][]string{reportAgentArgv(pane, name, state)}
}

// ReleaseArgv gives the pane back to herdr's own detection. release-agent REQUIRES --agent, so the
// name has to be carried here from the report: without it herdr exits 2 and the claim survives,
// which left every pane ever attached from still reporting an agent that had long since detached.
func ReleaseArgv(pane, name string) [][]string {
	return [][]string{
		{"pane", "release-agent", pane, "--source", source, "--agent", name},
		{"pane", "report-metadata", pane, "--source", source, "--clear-display-agent"},
	}
}

func reportAgentArgv(pane, name, state string) []string {
	return []string{"pane", "report-agent", pane, "--source", source, "--agent", name, "--state", state}
}

// Report claims the current pane for name.
func Report(name, state string) { run(ReportArgv(pane(), name, state)) }

// ReportState refreshes only the live state.
func ReportState(name, state string) { run(StateArgv(pane(), name, state)) }

// Release drops the claim on name. Takes the name because herdr will not release without it.
func Release(name string) { run(ReleaseArgv(pane(), name)) }

func pane() string { return os.Getenv("HERDR_PANE_ID") }

// run executes each argv, skipping everything when there is no pane to target. Failures are
// swallowed on purpose: a caller is mid-attach and often owns the screen, so writing there would
// corrupt it. Argv correctness is a test's job (-> herdr_test.go), not a runtime discovery.
func run(argvs [][]string) {
	if pane() == "" {
		return
	}
	for _, argv := range argvs {
		_ = exec.Command("herdr", argv...).Run()
	}
}

// State projects sindri's runtime substate onto herdr's vocabulary. On attach the agent is live, so
// an unknown runtime means working.
func State(runtime string) string {
	switch runtime {
	case "blocked":
		return "blocked"
	case "idle":
		return "idle"
	default: // "busy" or unknown
		return "working"
	}
}
