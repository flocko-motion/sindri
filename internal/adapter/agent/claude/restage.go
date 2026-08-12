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

// oauthEnvelope is the shape of that file, read for the fields that decide usability.
type oauthEnvelope struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

// tokenState is what one credential file says about its own usability.
type tokenState struct {
	token   string
	expires int64 // epoch ms; 0 when the file carries no expiry
}

// usable reports whether these credentials could authenticate anything. A file can be present,
// well-formed and worthless: an agent whose own refresh fails rewrites it with an EMPTY token and a
// zero expiry, which is what five agents were holding while the board called them signed out.
func (c tokenState) usable() bool { return c.token != "" && c.expires > 0 }

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
	host, err := parseCreds(hostData)
	if err != nil {
		return false, fmt.Errorf("read host credentials: %w", err)
	}
	// Never hand out what cannot work. The host's file is briefly blank while its own Claude Code
	// re-logs in, and copying it then would push a dead token onto every agent at once.
	if !host.usable() {
		return false, fmt.Errorf("the host's Claude credentials are not usable (no access token) — log in on the host")
	}
	dst := filepath.Join(dir, credFile)
	if have, rerr := os.ReadFile(dst); rerr == nil {
		// An unusable file is replaced whatever its expiry claims: the comparison below is about which
		// of two working tokens reaches further, and a blank one is not in that race.
		if mine, perr := parseCreds(have); perr == nil && mine.usable() && mine.expires >= host.expires {
			return false, nil // the agent's own is at least as fresh
		}
	}
	// In place, because the pod mounts this DIRECTORY: a write here is visible without a relaunch.
	if err := os.WriteFile(dst, hostData, 0o600); err != nil {
		return false, fmt.Errorf("restage credentials into %s: %w", dir, err)
	}
	return true, nil
}

// parseCreds reads a credential file's token and expiry. A file missing them parses fine and reports
// unusable — that IS the state to detect, so it is not an error.
func parseCreds(data []byte) (tokenState, error) {
	var env oauthEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return tokenState{}, err
	}
	return tokenState{token: env.ClaudeAiOauth.AccessToken, expires: env.ClaudeAiOauth.ExpiresAt}, nil
}

// HostTokenExpiry is when the host's own access token runs out, in epoch milliseconds, and whether
// the host has a usable one at all. The hub watches this to know when a redistribution is due:
// every agent shares this one token, so they all fall out of service the moment it lapses, and what
// decides whether they come back is how quickly its replacement reaches them.
func (Claude) HostTokenExpiry() (expiresAtMS int64, usable bool) {
	data, found := hostCredentials(io.Discard)
	if !found {
		return 0, false
	}
	c, err := parseCreds(data)
	if err != nil || !c.usable() {
		return 0, false
	}
	return c.expires, true
}
