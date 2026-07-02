// package: hub / token
// type:    logic (agent authentication for the TCP channel)
// job:     derive each agent's bearer token from one hub-global secret over its
//          (project, name), and resolve an incoming token back to that pair. Used
//          only by the macOS TCP agent channel (agenttcp.go), where — unlike a
//          per-agent unix socket — the endpoint can't be the identity.
// limits:  no transport here; agenttcp.go serves and hub.go hands the token to the
//          pod. The secret lives in the central store (hub.db), outside any repo.
package hub

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// tokenSecretKey names the hub-global secret in the store's meta table.
const tokenSecretKey = "agent_token_secret"

// agentSecret returns the hub-global secret that agent tokens are derived from,
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

// AgentToken derives an agent's bearer token: HMAC-SHA256(secret, project\x00name).
// Folding the project in means the same agent name in two repos gets distinct
// tokens. Deterministic given the hub secret, so it needs no per-agent storage and
// survives hub restarts.
func (h *Hub) AgentToken(project, name string) (string, error) {
	secret, err := h.agentSecret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(project + "\x00" + name))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// agentForToken resolves a bearer token to its (project, name), ok=false if none
// matches (constant-time compare per candidate). This is how the TCP channel turns
// a token into an identity, the way a unix socket's path does on Linux.
func (h *Hub) agentForToken(token string) (project, name string, ok bool) {
	if token == "" {
		return "", "", false
	}
	agents, err := h.store.AllAgents()
	if err != nil {
		return "", "", false
	}
	for _, a := range agents {
		want, err := h.AgentToken(a.Project, a.Name)
		if err == nil && hmac.Equal([]byte(want), []byte(token)) {
			return a.Project, a.Name, true
		}
	}
	return "", "", false
}
