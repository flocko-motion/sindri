// package: adapter/agent/claude / restage
// type:    adapter (Claude Code — credential upkeep)
// job:     carry the host's Claude credentials into a running agent's home when the
// host's reach further, so a re-login on the host reaches pods already up.
// limits:  compares expiry and copies; WHEN to call this is the hub's (-> credwatch).
package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// credFile is the name Claude reads its OAuth credentials from, in both homes.
const credFile = ".credentials.json"

// oauthEnvelope is the shape of that file, read for the one field this needs.
type oauthEnvelope struct {
	ClaudeAiOauth struct {
		ExpiresAt int64 `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

// RestageCredentials copies the host's credentials into dir when the host's access token outlasts
// the one already there, and reports whether it wrote.
//
// Expiry decides it, rather than the file's age: an agent refreshes its own token inside this very
// directory, so a copy driven by mtime would overwrite a token the pod had just renewed with an
// older one from the host. A token that reaches further is the only one worth writing.
func (Claude) RestageCredentials(dir string) (bool, error) {
	// An agent that has never launched has no home yet, and PrepareHome seeds one when it does.
	// Skipped rather than reported: absence here is the ordinary state of a registered agent.
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return false, nil
	}
	hostData, found := hostCredentials(io.Discard)
	if !found {
		return false, nil // nothing to carry; the host has no credentials to offer
	}
	hostExp, err := accessExpiry(hostData)
	if err != nil {
		return false, fmt.Errorf("read host credential expiry: %w", err)
	}
	dst := filepath.Join(dir, credFile)
	if have, rerr := os.ReadFile(dst); rerr == nil {
		if mine, perr := accessExpiry(have); perr == nil && mine >= hostExp {
			return false, nil // the agent's own is at least as fresh
		}
	}
	// In place, because the pod mounts this DIRECTORY: a write here is visible without a relaunch.
	if err := os.WriteFile(dst, hostData, 0o600); err != nil {
		return false, fmt.Errorf("restage credentials into %s: %w", dir, err)
	}
	return true, nil
}

// accessExpiry is the access token's expiry in epoch milliseconds.
func accessExpiry(data []byte) (int64, error) {
	var env oauthEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return 0, err
	}
	if env.ClaudeAiOauth.ExpiresAt == 0 {
		return 0, fmt.Errorf("no claudeAiOauth.expiresAt")
	}
	return env.ClaudeAiOauth.ExpiresAt, nil
}
