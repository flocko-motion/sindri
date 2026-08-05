package client

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/agentchan"
)

// testTimeout bounds the waits for the hub to come up and for an event to arrive.
// It's deliberately generous: a correct hub isn't slower under load, only later-
// scheduled, so a tight wall-clock deadline produces false failures on a busy
// machine (e.g. running the suite while agents work in parallel). Big enough to
// absorb scheduling starvation, small enough to still fail a genuine hang.
const testTimeout = 30 * time.Second

// serveTestHub starts a global hub isolated under a temp SINDRI_HOME (so its socket
// and state don't collide with the real hub or other tests) and waits for it.
func serveTestHub(t *testing.T) *hub.Hub {
	t.Helper()
	t.Setenv("SINDRI_HOME", t.TempDir())
	h, err := hub.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	go h.Serve()
	for deadline := time.Now().Add(testTimeout); !hub.IsRunning(); {
		if time.Now().After(deadline) {
			t.Fatal("hub never came up")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return h
}

// /events streams the initial snapshot and a fresh one after a mutation.
func TestWatchStreamsChanges(t *testing.T) {
	serveTestHub(t)
	root := t.TempDir() // the repo this client's requests concern (X-Sindri-Project)

	cl := Dial(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := cl.Watch(ctx)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	<-ch // initial snapshot (empty)

	if _, err := cl.NewAgent("brokkr", "worker", ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(testTimeout)
	for {
		select {
		case st := <-ch:
			if len(st.Agents) == 1 && st.Agents[0].Name == "brokkr" {
				return // change observed over SSE
			}
		case <-deadline:
			t.Fatal("never received the agent over /events")
		}
	}
}

func cmdNames(cmds []hub.CmdInfo) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Name
	}
	return out
}

func has(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// The agent socket IS the identity: a connection on brokkr's socket is brokkr.
// Exercises GET /commands (role-filtered) and POST /exec (streamed + exit) over a
// real per-agent socket.
func TestAgentSocketIdentityAndSurface(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir())
	h, err := hub.New()
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	const proj = "proj"

	// This exercises the Linux per-agent unix socket (on macOS agents use the TCP
	// channel — see TestAgentTCPChannelAuth). AF_UNIX paths are capped ~104 chars,
	// which a deep temp dir can exceed; skip rather than fail on that platform limit.
	if len(agentchan.SocketPath(proj, "brokkr")) > 100 {
		t.Skip("agent socket path exceeds the AF_UNIX length limit under this temp dir")
	}

	if _, err := h.NewAgent(proj, "brokkr", "worker", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.NewAgent(proj, "rune", "reviewer", ""); err != nil {
		t.Fatal(err)
	}
	if err := h.ServeAgent(proj, "brokkr"); err != nil {
		t.Fatal(err)
	}
	if err := h.ServeAgent(proj, "rune"); err != nil {
		t.Fatal(err)
	}

	worker := DialSocket(agentchan.SocketPath(proj, "brokkr"))
	rune := DialSocket(agentchan.SocketPath(proj, "rune"))

	// Idle worker surface: status/log/next; submit state-gated; approve/reject host-only.
	wc, err := worker.Commands()
	if err != nil {
		t.Fatalf("worker commands: %v", err)
	}
	wn := cmdNames(wc)
	for _, want := range []string{"status", "next"} {
		if !has(wn, want) {
			t.Fatalf("idle worker surface missing %q: %v", want, wn)
		}
	}
	for _, bad := range []string{"approve", "reject", "merge", "submit"} {
		if has(wn, bad) {
			t.Fatalf("idle worker surface must not include %q: %v", bad, wn)
		}
	}

	// Reviewer surface: approve/reject — never submit/next.
	rc, err := rune.Commands()
	if err != nil {
		t.Fatalf("reviewer commands: %v", err)
	}
	rn := cmdNames(rc)
	for _, want := range []string{"approve", "reject"} {
		if !has(rn, want) {
			t.Fatalf("reviewer surface missing %q: %v", want, rn)
		}
	}
	for _, bad := range []string{"submit", "next"} {
		if has(rn, bad) {
			t.Fatalf("reviewer surface must not include %q: %v", bad, rn)
		}
	}

	// Exec status over the worker socket → identity is brokkr/worker.
	var out bytes.Buffer
	exit, err := worker.Exec([]string{"status"}, &out)
	if err != nil || exit != 0 {
		t.Fatalf("status exec: exit=%d err=%v", exit, err)
	}
	if !strings.Contains(out.String(), "agent:   brokkr") || !strings.Contains(out.String(), "role:    worker") {
		t.Fatalf("status output wrong: %q", out.String())
	}

	// A reviewer-only verb is invisible to the worker → "unknown or unavailable".
	out.Reset()
	exit, _ = worker.Exec([]string{"approve"}, &out)
	if exit != 127 || !strings.Contains(out.String(), "unknown or unavailable") {
		t.Fatalf("worker should not run approve: exit=%d out=%q", exit, out.String())
	}
}

// Exercise the full socket path end to end: hub server + HTTP client + store, for
// the podman-free operations (register + state).
func TestServeAndClientRoundTrip(t *testing.T) {
	serveTestHub(t)
	root := t.TempDir()

	cl := Dial(root)
	if _, err := cl.NewAgent("brokkr", "worker", ""); err != nil {
		t.Fatalf("client NewAgent: %v", err)
	}
	st, err := cl.State()
	if err != nil {
		t.Fatalf("client State: %v", err)
	}
	if len(st.Agents) != 1 || st.Agents[0].Name != "brokkr" {
		t.Fatalf("unexpected state over socket: %+v", st)
	}
	// The hub's domain error must surface across the socket.
	if _, err := cl.NewAgent("brokkr", "worker", ""); err == nil {
		t.Fatalf("expected duplicate error over socket")
	}
}
