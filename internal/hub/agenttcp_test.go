package hub

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAgentTCPChannelAuth exercises the macOS agent channel end to end at the HTTP
// layer: the listener comes up on loopback, a valid token authenticates and routes
// into the agent surface, and a missing or bad token is rejected with 401.
func TestAgentTCPChannelAuth(t *testing.T) {
	h, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if err := h.store.PutAgent(store.Agent{Name: "eitri", Role: "coauthor"}); err != nil {
		t.Fatal(err)
	}
	if err := h.serveAgentTCP(); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("http://127.0.0.1:%d/commands", h.agentTCPPort)
	tok, _ := h.AgentToken("eitri")

	do := func(token string) int {
		req, _ := http.NewRequest("GET", base, nil)
		if token != "" {
			req.Header.Set("X-Sindri-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := do(tok); code == http.StatusUnauthorized {
		t.Error("valid token was rejected")
	}
	if code := do(""); code != http.StatusUnauthorized {
		t.Errorf("missing token: got %d, want 401", code)
	}
	if code := do("deadbeef"); code != http.StatusUnauthorized {
		t.Errorf("bad token: got %d, want 401", code)
	}
}
