// package: hub / token
// type:    logic (agent authentication for the TCP channel)
// job:     derive each agent's bearer token from one per-repo secret, and resolve
//          an incoming token back to its agent. Used only by the macOS TCP agent
//          channel (agenttcp.go), where — unlike a per-agent unix socket — the
//          endpoint can't be the identity, so a token must be.
// limits:  no transport here; agenttcp.go serves and hub.go hands the token to the
//          pod. The secret lives in the store (hub.db), which is gitignored.
package hub

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// tokenSecretKey names the per-repo secret in the store's meta table.
const tokenSecretKey = "agent_token_secret"

// agentSecret returns the per-repo secret that agent tokens are derived from,
// generating and persisting it on first use. Persisting it (rather than keeping it
// in memory) keeps tokens stable across hub restarts, so a restarted hub still
// authenticates already-running pods (D11).
func (h *Hub) agentSecret() ([]byte, error) {
	if v, ok, err := h.store.GetMeta(tokenSecretKey); err != nil {
		return nil, err
	} else if ok && v != "" {
		return hex.DecodeString(v)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate agent secret: %w", err)
	}
	if err := h.store.SetMeta(tokenSecretKey, hex.EncodeToString(buf)); err != nil {
		return nil, err
	}
	return buf, nil
}

// AgentToken derives agent name's bearer token: HMAC-SHA256(secret, name). It's
// deterministic given the repo secret, so the same agent always gets the same
// token — no per-agent storage needed, and it survives hub restarts.
func (h *Hub) AgentToken(name string) (string, error) {
	secret, err := h.agentSecret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(name))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// agentForToken returns the rostered agent whose token matches (constant-time),
// ok=false if none does. This is how the TCP channel turns a bearer token into an
// identity, the way a unix socket's path does on Linux.
func (h *Hub) agentForToken(token string) (name string, ok bool) {
	if token == "" {
		return "", false
	}
	roster, err := h.store.Roster()
	if err != nil {
		return "", false
	}
	for _, a := range roster {
		want, err := h.AgentToken(a.Name)
		if err == nil && hmac.Equal([]byte(want), []byte(token)) {
			return a.Name, true
		}
	}
	return "", false
}
