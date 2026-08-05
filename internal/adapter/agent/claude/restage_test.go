package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// creds writes a credential file whose access token expires at the given offset from now.
func creds(t *testing.T, path string, in time.Duration) {
	t.Helper()
	body := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok","refreshToken":"ref","expiresAt":%d}}`,
		time.Now().Add(in).UnixMilli())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// expiryOf reads back what a home now holds.
func expiryOf(t *testing.T, path string) int64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var env oauthEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return env.ClaudeAiOauth.ExpiresAt
}

// TestARenewedHostTokenReachesTheAgent is the point: a re-login on the host has to arrive without
// relaunching the pod, which mounts this directory.
func TestARenewedHostTokenReachesTheAgent(t *testing.T) {
	hostDir := hostHome(t)
	creds(t, filepath.Join(hostDir, credFile), 6*time.Hour)
	dir := t.TempDir()
	creds(t, filepath.Join(dir, credFile), 10*time.Minute) // the agent's, nearly out

	wrote, err := Claude{}.RestageCredentials(dir)
	if err != nil {
		t.Fatalf("RestageCredentials: %v", err)
	}
	if !wrote {
		t.Fatal("a host token reaching further must be carried over")
	}
	if got, want := expiryOf(t, filepath.Join(dir, credFile)), expiryOf(t, filepath.Join(hostDir, credFile)); got != want {
		t.Errorf("agent expiry = %d, want the host's %d", got, want)
	}
}

// TestAFresherAgentTokenIsLeftAlone is the hazard the expiry comparison exists for: the pod renews
// its own token inside this directory, so copying on age would replace a new token with an old one.
func TestAFresherAgentTokenIsLeftAlone(t *testing.T) {
	hostDir := hostHome(t)
	creds(t, filepath.Join(hostDir, credFile), 30*time.Minute) // the host's, older
	dir := t.TempDir()
	agentPath := filepath.Join(dir, credFile)
	creds(t, agentPath, 6*time.Hour) // the pod refreshed its own
	before := expiryOf(t, agentPath)

	wrote, err := Claude{}.RestageCredentials(dir)
	if err != nil {
		t.Fatalf("RestageCredentials: %v", err)
	}
	if wrote {
		t.Error("a token the agent renewed itself must not be replaced by an older host one")
	}
	if after := expiryOf(t, agentPath); after != before {
		t.Errorf("expiry changed from %d to %d", before, after)
	}
}

// TestAnAgentWithNoCredentialsIsSeeded covers the homes that hold a snapshot too old to refresh, or
// none at all: there is nothing to protect, so the host's are written.
func TestAnAgentWithNoCredentialsIsSeeded(t *testing.T) {
	hostDir := hostHome(t)
	creds(t, filepath.Join(hostDir, credFile), 6*time.Hour)
	dir := t.TempDir()

	wrote, err := Claude{}.RestageCredentials(dir)
	if err != nil {
		t.Fatalf("RestageCredentials: %v", err)
	}
	if !wrote {
		t.Fatal("an empty home must be seeded")
	}
	if expiryOf(t, filepath.Join(dir, credFile)) == 0 {
		t.Error("the seeded file should carry the host's expiry")
	}
}

// TestNoHostCredentialsChangesNothing: a host without credentials has nothing to offer, and must not
// clear what an agent already holds.
func TestNoHostCredentialsChangesNothing(t *testing.T) {
	hostHome(t) // no .claude/.credentials.json written
	dir := t.TempDir()
	agentPath := filepath.Join(dir, credFile)
	creds(t, agentPath, 6*time.Hour)
	before := expiryOf(t, agentPath)

	wrote, err := Claude{}.RestageCredentials(dir)
	if err != nil {
		t.Fatalf("RestageCredentials: %v", err)
	}
	if wrote {
		t.Error("with nothing on the host, nothing may be written")
	}
	if after := expiryOf(t, agentPath); after != before {
		t.Errorf("the agent's own credentials were disturbed: %d then %d", before, after)
	}
}

// TestAnAgentThatNeverLaunchedIsSkipped: a registered agent has no home until it starts, and the
// sweep runs over the whole roster — so absence is the ordinary case rather than a fault to report.
func TestAnAgentThatNeverLaunchedIsSkipped(t *testing.T) {
	hostDir := hostHome(t)
	creds(t, filepath.Join(hostDir, credFile), 6*time.Hour)
	missing := filepath.Join(t.TempDir(), "never-launched")

	wrote, err := Claude{}.RestageCredentials(missing)
	if err != nil {
		t.Errorf("a home that does not exist yet must not be an error: %v", err)
	}
	if wrote {
		t.Error("nothing may be written where no home exists")
	}
	if _, serr := os.Stat(missing); serr == nil {
		t.Error("the sweep must not create a home; PrepareHome does that at launch")
	}
}

// TestAnEqualExpiryIsNotRewritten keeps the sweep quiet: five minutes apart, nothing has changed, and
// a write every pass would churn a file the pod is reading.
func TestAnEqualExpiryIsNotRewritten(t *testing.T) {
	hostDir := hostHome(t)
	hostPath := filepath.Join(hostDir, credFile)
	creds(t, hostPath, 6*time.Hour)
	dir := t.TempDir()
	data, err := os.ReadFile(hostPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, credFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	wrote, err := Claude{}.RestageCredentials(dir)
	if err != nil {
		t.Fatalf("RestageCredentials: %v", err)
	}
	if wrote {
		t.Error("an unchanged credential must not be rewritten")
	}
}
