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

// Report labels the current herdr pane with agent `name` in herdr state `state`
// (working|blocked|idle|done|unknown), sourced as "sindri", so herdr's sidebar shows
// the agent by its own name (e.g. "austri") rather than the generic "claude".
// Best-effort: a no-op outside herdr, errors ignored (it must not affect the attach).
func Report(name, state string) {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	_ = exec.Command("herdr", "pane", "report-agent", pane,
		"--source", "sindri", "--agent", name, "--state", state).Run()
}

// Release drops sindri's authority over the pane's sidebar entry on detach, so herdr
// falls back to its own detection for what is now a plain shell. Best-effort.
func Release() {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	_ = exec.Command("herdr", "pane", "release-agent", pane, "--source", "sindri").Run()
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
