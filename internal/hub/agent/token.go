// package: hub/agent / token
// type:    logic (agent authentication for the TCP channel)
// job:     derive each agent's bearer token from one hub-global secret over its
//          (project, name), and resolve an incoming token back to that pair. Used
//          only by the macOS TCP agent channel (agenttcp.go), where — unlike a
//          per-agent unix socket — the endpoint can't be the identity.
// limits:  no transport here; the hub serves the channel and hands the token to the
//          pod. The secret lives in the central store (hub.db), outside any repo.
package agent

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// tokenSecretKey names the hub-global secret in the store's meta table.
const tokenSecretKey = "agent_token_secret"

// secret returns the hub-global secret that agent tokens are derived from,
// generating and persisting it on first use. Persisting it (rather than keeping it
// in memory) keeps tokens stable across hub restarts, so a restarted hub still
// authenticates already-running pods (D11).
func (s *Service) secret() ([]byte, error) {
	if v, ok, err := s.store.GetMeta(tokenSecretKey); err != nil {
		return nil, err
	} else if ok && v != "" {
		return hex.DecodeString(v)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate agent secret: %w", err)
	}
	if err := s.store.SetMeta(tokenSecretKey, hex.EncodeToString(buf)); err != nil {
		return nil, err
	}
	return buf, nil
}

// Token derives an agent's bearer token: HMAC-SHA256(secret, project\x00name).
// Folding the project in means the same agent name in two repos gets distinct
// tokens. Deterministic given the hub secret, so it needs no per-agent storage and
// survives hub restarts.
func (s *Service) Token(project, name string) (string, error) {
	secret, err := s.secret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(project + "\x00" + name))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// ForToken resolves a bearer token to its (project, name), ok=false if none matches
// (constant-time compare per candidate). This is how the TCP channel turns a token
// into an identity, the way a unix socket's path does on Linux. A store failure is
// returned as err (distinct from ok=false), so the caller can answer 500 rather than
// disguising a DB outage as "bad token".
func (s *Service) ForToken(token string) (project, name string, ok bool, err error) {
	if token == "" {
		return "", "", false, nil
	}
	agents, err := s.store.AllAgents()
	if err != nil {
		return "", "", false, fmt.Errorf("load agents for token auth: %w", err)
	}
	for _, a := range agents {
		want, terr := s.Token(a.Project, a.Name)
		if terr == nil && hmac.Equal([]byte(want), []byte(token)) {
			return a.Project, a.Name, true, nil
		}
	}
	return "", "", false, nil
}
