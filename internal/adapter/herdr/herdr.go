// package: adapter/herdr / herdr
// type:    adapter (external tool: herdr, the agent multiplexer)
// job:     when `sindri agent attach` runs inside a herdr pane, report the pane to
//          herdr's sidebar under the agent's own name (e.g. "austri"), sourced as
//          "sindri" (the reporting tool), with a live state — so the agent shows up
//          there as if it ran natively.
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

// Report labels the current herdr pane as this agent for the sidebar and every
// notification: the agent field carries the agent's own name (e.g. "austri") — herdr
// renders it in both the sidebar row AND its attention/finished toasts, so the name
// must live there, not only in the display override. --source stays "sindri" (the
// tool reporting it, not "claude"). Two calls: report-agent carries the name + live
// state; report-metadata re-asserts the display name so it wins even over herdr's own
// detection (which would otherwise label the pane "claude"). Best-effort — a no-op
// outside herdr, errors ignored so it never disturbs the attach.
func Report(name, state string) {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	reportAgent(pane, name, state)
	_ = exec.Command("herdr", "pane", "report-metadata", pane,
		"--source", "sindri", "--agent", name, "--display-agent", name).Run()
}

// ReportState refreshes only the live state (report-agent), for the periodic updates
// that keep herdr current during a long attach — the display metadata is already set
// by the initial Report, so there's no need to re-send it each tick. Best-effort.
func ReportState(name, state string) {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	reportAgent(pane, name, state)
}

// reportAgent tells herdr the pane is this named agent in the given state. The name
// goes in --agent (herdr's authoritative label for both sidebar and toasts).
func reportAgent(pane, name, state string) {
	_ = exec.Command("herdr", "pane", "report-agent", pane,
		"--source", "sindri", "--agent", name, "--state", state).Run()
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
