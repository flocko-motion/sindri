package herdr

import (
	"slices"
	"testing"
)

// TestReleaseNamesTheAgent is the bug this file exists for: `herdr pane release-agent` requires
// --agent, and sindri sent only --source. herdr exited 2, the error was discarded, and the claim
// outlived the attach — so every pane ever attached from kept reporting an agent long after detach.
func TestReleaseNamesTheAgent(t *testing.T) {
	argvs := ReleaseArgv("w3:p16", "bombur")
	if len(argvs) != 2 {
		t.Fatalf("want 2 argvs (release + clear display), got %d: %q", len(argvs), argvs)
	}
	want := []string{"pane", "release-agent", "w3:p16", "--source", "sindri", "--agent", "bombur"}
	if !slices.Equal(argvs[0], want) {
		t.Fatalf("release argv wrong:\n got %q\nwant %q", argvs[0], want)
	}
	// The display name is sindri's too, so releasing the claim has to give that back as well.
	wantClear := []string{"pane", "report-metadata", "w3:p16", "--source", "sindri", "--clear-display-agent"}
	if !slices.Equal(argvs[1], wantClear) {
		t.Fatalf("clear-display argv wrong:\n got %q\nwant %q", argvs[1], wantClear)
	}
}

// TestReportClaimsAgentAndDisplay: the agent field drives the sidebar and toasts, and the display
// name has to be pinned separately or herdr's own detection relabels the pane "claude".
func TestReportClaimsAgentAndDisplay(t *testing.T) {
	argvs := ReportArgv("w3:p16", "bombur", "working")
	if len(argvs) != 2 {
		t.Fatalf("want 2 argvs (agent + metadata), got %d: %q", len(argvs), argvs)
	}
	want := []string{"pane", "report-agent", "w3:p16", "--source", "sindri", "--agent", "bombur", "--state", "working"}
	if !slices.Equal(argvs[0], want) {
		t.Fatalf("report argv wrong:\n got %q\nwant %q", argvs[0], want)
	}
	wantMeta := []string{"pane", "report-metadata", "w3:p16", "--source", "sindri", "--agent", "bombur", "--display-agent", "bombur"}
	if !slices.Equal(argvs[1], wantMeta) {
		t.Fatalf("metadata argv wrong:\n got %q\nwant %q", argvs[1], wantMeta)
	}
}

// TestStateRefreshCarriesEveryRequiredFlag: report-agent requires --source, --agent and --state on
// every call, including the periodic refresh, which sends nothing else.
func TestStateRefreshCarriesEveryRequiredFlag(t *testing.T) {
	argvs := StateArgv("w3:p16", "bombur", "idle")
	if len(argvs) != 1 {
		t.Fatalf("a refresh is one command, got %d: %q", len(argvs), argvs)
	}
	for _, required := range []string{"--source", "--agent", "--state"} {
		if !slices.Contains(argvs[0], required) {
			t.Errorf("refresh argv is missing %s: %q", required, argvs[0])
		}
	}
}

// TestEveryArgvNamesSourceAndPane: herdr keys a claim by (pane, source), so a command that loses
// either targets the wrong thing or is rejected.
func TestEveryArgvNamesSourceAndPane(t *testing.T) {
	all := ReportArgv("w3:p1", "dain", "working")
	all = append(all, StateArgv("w3:p1", "dain", "blocked")...)
	all = append(all, ReleaseArgv("w3:p1", "dain")...)
	for _, argv := range all {
		if !slices.Contains(argv, "w3:p1") {
			t.Errorf("argv does not target the pane: %q", argv)
		}
		if !slices.Contains(argv, "--source") || !slices.Contains(argv, "sindri") {
			t.Errorf("argv does not name sindri as the source: %q", argv)
		}
	}
}

// TestStateMapsUnknownToWorking: an attach means the agent is live, so a failed probe must not
// report it idle and have herdr show it as finished.
func TestStateMapsUnknownToWorking(t *testing.T) {
	for runtime, want := range map[string]string{
		"blocked": "blocked",
		"idle":    "idle",
		"busy":    "working",
		"":        "working",
		"garbage": "working",
	} {
		if got := State(runtime); got != want {
			t.Errorf("State(%q) = %q, want %q", runtime, got, want)
		}
	}
}
