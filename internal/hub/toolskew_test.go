package hub

import (
	"context"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// defaultPodManifest must stay empty when nothing exists on EITHER of its two sources — a fresh
// install with no image built and no pod-bin/brokkr synced — not an empty-but-non-nil map, which
// would silently defeat check()'s "nothing yet to compare" guard. Isolates XDG_CACHE_HOME/HOME too
// (container.ImageManifest reads os.UserCacheDir(), not SINDRI_HOME), so this cannot read a real
// manifest a developer's own machine happens to have built.
func TestDefaultPodManifestEmptyWhenNothingBuiltOrSynced(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	m, err := defaultPodManifest()
	if err != nil {
		t.Fatalf("defaultPodManifest: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("want nothing yet, got %v", m)
	}
}

// newHubWithTools is newHub, but with toolskew's two lookups replaced by fixed stand-ins — used only
// by this file's own tests, which are the one place they need to be anything but a no-op.
func newHubWithTools(t *testing.T, host map[string]string, manifest map[string]string, manifestErr error) *Hub {
	t.Helper()
	t.Setenv("SINDRI_HOME", t.TempDir())
	h, err := open(t.Context(),
		func(context.Context) map[string]string { return host },
		func() (map[string]string, error) { return manifest, manifestErr })
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func userMail(t *testing.T, h *Hub) []string {
	t.Helper()
	mail, err := h.store.For(workflow.GlobalProject).UnreadMail(api.SenderUser)
	if err != nil {
		t.Fatalf("UnreadMail: %v", err)
	}
	bodies := make([]string, len(mail))
	for i, m := range mail {
		bodies[i] = m.Body
	}
	return bodies
}

// No image has ever been built here: there is nothing yet to compare, which must stay silent
// rather than read as a mismatch.
func TestToolSkewSilentWithNoManifestYet(t *testing.T) {
	h := newHubWithTools(t, map[string]string{"go": "go1.27.0"}, nil, nil)
	if mail := userMail(t, h); len(mail) != 0 {
		t.Errorf("no manifest yet should mail nothing, got %v", mail)
	}
}

// Matching versions on both sides is the healthy case — nothing to tell the user.
func TestToolSkewSilentWhenVersionsMatch(t *testing.T) {
	h := newHubWithTools(t,
		map[string]string{"go": "go1.27.0", "node": "v22.11.0"},
		map[string]string{"go": "go1.27.0", "node": "v22.11.0"},
		nil)
	if mail := userMail(t, h); len(mail) != 0 {
		t.Errorf("matching versions should mail nothing, got %v", mail)
	}
}

// The one case this whole task is about: pods and host disagree, and the user must be able to
// read it from their mailbox rather than a startup log line that scrolls away unread.
func TestToolSkewMailsUserOnMismatch(t *testing.T) {
	h := newHubWithTools(t,
		map[string]string{"go": "go1.27.0", "openspec": "1.4.1"},
		map[string]string{"go": "go1.26.4", "openspec": "1.8.0"},
		nil)
	mail := userMail(t, h)
	if len(mail) != 1 {
		t.Fatalf("want exactly one mail about the mismatch, got %v", mail)
	}
	body := mail[0]
	for _, want := range []string{"go", "go1.26.4", "go1.27.0", "openspec", "1.8.0", "1.4.1"} {
		if !strings.Contains(body, want) {
			t.Errorf("mismatch mail should mention %q: %q", want, body)
		}
	}
}

// A tool present on only one side (e.g. the host lacks openspec on PATH) is not a mismatch to
// report — there is nothing to compare it against, not a disagreement.
func TestToolSkewIgnoresAToolMissingFromTheHostSide(t *testing.T) {
	h := newHubWithTools(t,
		map[string]string{"go": "go1.27.0"}, // no "openspec" entry at all
		map[string]string{"go": "go1.27.0", "openspec": "1.8.0"},
		nil)
	if mail := userMail(t, h); len(mail) != 0 {
		t.Errorf("a tool missing on the host side should not be reported as a mismatch, got %v", mail)
	}
}

// The credwatch.say discipline this task explicitly asks for: the same finding, checked twice,
// must reach the user's mailbox once — within one process (-> TestToolSkewSurvivesRestart for the
// actual-restart case, which is what the requirement is really about).
func TestToolSkewSaysOnceUntilItChanges(t *testing.T) {
	h := newHubWithTools(t, map[string]string{"go": "go1.27.0"}, map[string]string{"go": "go1.26.4"}, nil)
	if mail := userMail(t, h); len(mail) != 1 {
		t.Fatalf("first check: want one mail, got %v", mail)
	}
	h.tools.check() // same finding again — must not mail a second time
	if mail := userMail(t, h); len(mail) != 1 {
		t.Fatalf("repeat of the same finding must not mail again, got %v", mail)
	}
	// The skew resolves: the next distinct finding (here, none at all) must be free to report again.
	h.tools.podManifest = func() (map[string]string, error) { return map[string]string{"go": "go1.27.0"}, nil }
	h.tools.check()
	h.tools.podManifest = func() (map[string]string, error) { return map[string]string{"go": "go1.26.4"}, nil }
	h.tools.check()
	if mail := userMail(t, h); len(mail) != 2 {
		t.Fatalf("a finding that cleared and came back should mail again, got %v", mail)
	}
}

// A failed send must not be recorded as reported: SAY IT ONCE is about not repeating a mismatch the
// user HAS read, not about giving up on one they never received. Forcing the mail write to fail
// (closing the store) must leave said unset, so the very next check retries rather than staying
// silent for ever.
func TestToolSkewRetriesAfterAFailedSend(t *testing.T) {
	h := newHubWithTools(t, map[string]string{"go": "go1.27.0"}, map[string]string{"go": "go1.27.0"}, nil)
	if h.tools.said != "" {
		t.Fatalf("matching versions at construction should leave said empty, got %q", h.tools.said)
	}
	if err := h.store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	h.tools.podManifest = func() (map[string]string, error) { return map[string]string{"go": "go1.26.4"}, nil }
	h.tools.check() // the mail write must fail now the store is closed
	if h.tools.said != "" {
		t.Errorf("a failed send must not be recorded as said, got %q", h.tools.said)
	}
}

// The requirement this task actually names: a RESTART, not just a second check() in the same
// process, must not repeat a mismatch already mailed. Pinning one SINDRI_HOME across two sequential
// Hubs, closing the first before opening the second, is what a real restart is.
func TestToolSkewSurvivesRestart(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir())
	hostVersions := func(context.Context) map[string]string { return map[string]string{"go": "go1.27.0"} }
	podManifest := func() (map[string]string, error) { return map[string]string{"go": "go1.26.4"}, nil }

	h1, err := open(t.Context(), hostVersions, podManifest)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if mail := userMail(t, h1); len(mail) != 1 {
		t.Fatalf("first startup: want one mail, got %v", mail)
	}
	if err := h1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	h2, err := open(t.Context(), hostVersions, podManifest)
	if err != nil {
		t.Fatalf("second open (the restart): %v", err)
	}
	t.Cleanup(func() { h2.Close() })
	if mail := userMail(t, h2); len(mail) != 1 {
		t.Errorf("restart with the same mismatch must not mail again, got %v", mail)
	}
}
