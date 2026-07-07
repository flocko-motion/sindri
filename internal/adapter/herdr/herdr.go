// package: adapter/herdr / herdr
// type:    adapter (external tool: herdr, the agent multiplexer)
// job:     when `sindri agent attach` runs inside a herdr pane, report the pane to
//          herdr's sidebar (by the agent's own name, e.g. "austri", sourced as
//          "sindri" so it's never mislabelled "claude") with a projected state — so
//          the agent shows up there as if it ran natively.
// limits:  optional UI nicety, best-effort — a no-op outside herdr, and any failure
//          is swallowed so it never disturbs the terminal handover. The `herdr`
//          binary + HERDR_* env are present only inside a herdr pane.
package herdr

import (
	"os"
	"os/exec"
)

// InPane reports whether we're running inside a herdr-managed pane — HERDR_ENV=1 plus
// a pane id to target (both are set by herdr on every pane).
func InPane() bool {
	return os.Getenv("HERDR_ENV") == "1" && os.Getenv("HERDR_PANE_ID") != ""
}

// Report labels the current herdr pane as a sindri agent for the sidebar: the kind
// is "sindri" (the identity — not "claude", which is just today's config) and the
// display name is the agent's own name (e.g. "austri", which the user relates to),
// with the live state (working|blocked|idle|done|unknown). Two calls: report-agent
// carries the kind + state, report-metadata the display name. Best-effort — a no-op
// outside herdr, errors ignored so it never disturbs the attach.
func Report(name, state string) {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	_ = exec.Command("herdr", "pane", "report-agent", pane,
		"--source", "sindri", "--agent", "sindri", "--state", state).Run()
	_ = exec.Command("herdr", "pane", "report-metadata", pane,
		"--source", "sindri", "--agent", "sindri", "--display-agent", name).Run()
}

// Release drops sindri's sidebar authority over the pane on detach — clearing both
// the agent report and the display name — so herdr falls back to its own detection
// for what is now a plain shell. Best-effort.
func Release() {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	_ = exec.Command("herdr", "pane", "release-agent", pane, "--source", "sindri").Run()
	_ = exec.Command("herdr", "pane", "report-metadata", pane,
		"--source", "sindri", "--clear-display-agent").Run()
}

// State projects sindri's runtime substate (busy|blocked|idle|"") onto herdr's agent
// vocabulary. On attach the agent is live, so an unknown runtime maps to "working".
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
