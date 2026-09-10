package main

import (
	"slices"
	"testing"
)

// names is the table's linters in order, which is what runAll runs and what its summary lists.
func names(o lintOpts) []string {
	var out []string
	for _, l := range lintersFor(o) {
		out = append(out, l.name)
	}
	return out
}

// TestAnUnaskedConventionDoesNotRun is the rule itself: brokkr is a toolbelt pointed at any repo,
// so a linter holding a convention this project never adopted must stay out of the run — otherwise
// a repo that names its tests its own way fails a gate it never agreed to.
func TestAnUnaskedConventionDoesNotRun(t *testing.T) {
	got := names(lintOpts{})
	for _, n := range optInNames() {
		if slices.Contains(got, n) {
			t.Errorf("%s ran with nothing enabled: %v", n, got)
		}
	}
	// The rest are properties of Go, so they hold in every repo with no config at all.
	for _, n := range []string{"deadcode", "loc", "comments", "comment-length", "gofmt", "js", "openspec"} {
		if !slices.Contains(got, n) {
			t.Errorf("%s must run everywhere, got %v", n, got)
		}
	}
}

// TestAnAskedConventionRuns: the enable list is the whole opt-in, and it turns on exactly what it
// names — a repo adopting one convention has not adopted the other two.
func TestAnAskedConventionRuns(t *testing.T) {
	got := names(lintOpts{enabled: map[string]bool{"test-home": true}})
	if !slices.Contains(got, "test-home") {
		t.Errorf("an enabled linter must run: %v", got)
	}
	for _, n := range []string{"header-path", "verb-help"} {
		if slices.Contains(got, n) {
			t.Errorf("enabling test-home also ran %s: %v", n, got)
		}
	}
}

// TestANameOutsideTheOptInSetIsRefused: the two ways to write one are a typo and a linter that
// already runs everywhere, and both leave the repo believing it asked for something.
func TestANameOutsideTheOptInSetIsRefused(t *testing.T) {
	for _, n := range []string{"test-hom", "loc", ""} {
		if _, err := enabledLinters([]string{n}); err == nil {
			t.Errorf("lint.enable: %q was accepted", n)
		}
	}
	on, err := enabledLinters([]string{"verb-help", "header-path"})
	if err != nil {
		t.Fatalf("enabledLinters: %v", err)
	}
	if len(on) != 2 || !on["verb-help"] || !on["header-path"] {
		t.Errorf("enabled set = %v", on)
	}
}

// TestEveryOptInLinterIsALinter guards the two lists drifting: a name in optIn that splitLintArgs
// does not know is a linter nobody can run by hand, and one lintersFor never offers is dead.
func TestEveryOptInLinterIsALinter(t *testing.T) {
	all := names(lintOpts{enabled: optInSet()})
	for _, n := range optInNames() {
		if !lintNames[n] {
			t.Errorf("%s is opt-in but not a linter name", n)
		}
		if !slices.Contains(all, n) {
			t.Errorf("%s is opt-in but in no run table: %v", n, all)
		}
	}
}

// optInSet enables every opt-in linter, for the guard above.
func optInSet() map[string]bool {
	on := map[string]bool{}
	for _, n := range optInNames() {
		on[n] = true
	}
	return on
}
