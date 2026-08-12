package goplsmcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// converse drives serveAdvice over one batch of requests and returns the decoded responses.
func converse(t *testing.T, advice string, reqs ...string) []map[string]any {
	t.Helper()
	var out strings.Builder
	if err := serveAdvice(advice, strings.NewReader(strings.Join(reqs, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var got []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("undecodable response %q: %v", line, err)
		}
		got = append(got, m)
	}
	return got
}

const advice = "Go toolchain too old: this module needs go 1.99.0 but go 1.26.5 is running — run `go-upgrade` to install it, then re-run"

// TestToolCallIsAnErrorNotAnEmptyAnswer is the whole reason this shim exists. With a refused
// toolchain gopls loads no packages and go_search answers "No symbols found." — which reads as a
// fact about the code, so an agent concludes the symbol does not exist and acts on it. Every tool
// must instead come back flagged as an error, carrying the reason and the fix.
func TestToolCallIsAnErrorNotAnEmptyAnswer(t *testing.T) {
	got := converse(t, advice,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"go_search","arguments":{"query":"F"}}}`)
	if len(got) != 1 {
		t.Fatalf("want 1 response, got %d", len(got))
	}
	res, _ := got[0]["result"].(map[string]any)
	if res["isError"] != true {
		t.Error("a tool call succeeded — the client will read the answer as authoritative")
	}
	body := text(t, res)
	if !strings.Contains(body, "1.99.0") || !strings.Contains(body, "go-upgrade") {
		t.Errorf("the answer carries neither the version needed nor the fix: %q", body)
	}
	// The failure it replaces must not be imitated: nothing may read as a finding about the code.
	if strings.Contains(strings.ToLower(body), "no symbols found") {
		t.Errorf("the shim reproduced the misleading empty answer: %q", body)
	}
}

// TestEveryGoplsToolAnswers: the agent reaches for a tool by name, so the shim must answer under
// all of gopls' names. A server offering none would look like an integration that failed to load,
// which says nothing about go.mod.
func TestEveryGoplsToolAnswers(t *testing.T) {
	got := converse(t, advice, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	res, _ := got[0]["result"].(map[string]any)
	listed := map[string]bool{}
	for _, raw := range res["tools"].([]any) {
		tool := raw.(map[string]any)
		listed[tool["name"].(string)] = true
		if !strings.Contains(tool["description"].(string), "UNAVAILABLE") {
			t.Errorf("%s does not announce itself as unavailable", tool["name"])
		}
	}
	for _, want := range Tools {
		if !listed[want] {
			t.Errorf("%s is not offered, so reaching for it gets an unknown-tool error instead of the reason", want)
		}
	}
	// And each name must actually answer when called, not merely be listed.
	for _, name := range Tools {
		call := `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"` + name + `"}}`
		r, _ := converse(t, advice, call)[0]["result"].(map[string]any)
		if r["isError"] != true {
			t.Errorf("%s answered without an error flag", name)
		}
	}
}

// TestNotificationsAreNotAnswered: a JSON-RPC notification has no id, and replying to one is a
// protocol error that can wedge the client.
func TestNotificationsAreNotAnswered(t *testing.T) {
	got := converse(t, advice,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if len(got) != 1 {
		t.Fatalf("want exactly 1 response (the notification must not be answered), got %d", len(got))
	}
	if id, _ := got[0]["id"].(float64); id != 1 {
		t.Errorf("the reply is not to the request that had an id: %v", got[0]["id"])
	}
}

// TestInitializeEchoesTheClientsProtocolVersion: pinning a version this shim invented would be
// refused by a client that has moved on, and gopls itself echoes.
func TestInitializeEchoesTheClientsProtocolVersion(t *testing.T) {
	got := converse(t, advice,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2031-01-01"}}`)
	res, _ := got[0]["result"].(map[string]any)
	if res["protocolVersion"] != "2031-01-01" {
		t.Errorf("protocolVersion = %v, want the client's own", res["protocolVersion"])
	}
	if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
		t.Error("the server does not declare tool capability, so no tool is ever called")
	}
	// A client that sends no version must still get one back, or it cannot proceed.
	bare := converse(t, advice, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	bres, _ := bare[0]["result"].(map[string]any)
	if bres["protocolVersion"] == "" || bres["protocolVersion"] == nil {
		t.Error("a client that named no protocol version got none back")
	}
}

// TestProbeIsSilentOnAHealthyModule: this repo builds, so the shim must delegate rather than
// declare a problem. A false positive here would replace working Go tools with an error message.
func TestProbeIsSilentOnAHealthyModule(t *testing.T) {
	if got := Probe("../../.."); got != "" {
		t.Errorf("the probe reported a toolchain refusal in a module that builds: %q", got)
	}
}

// TestProbeIgnoresFailuresItDoesNotUnderstand: a directory that is no module at all fails the probe
// command too, and claiming a toolchain problem there would be a fabricated diagnosis.
func TestProbeIgnoresFailuresItDoesNotUnderstand(t *testing.T) {
	if got := Probe(t.TempDir()); got != "" {
		t.Errorf("a non-module directory was diagnosed as a toolchain refusal: %q", got)
	}
}

func text(t *testing.T, res map[string]any) string {
	t.Helper()
	content, _ := res["content"].([]any)
	if len(content) == 0 {
		t.Fatal("the response carries no content — an empty box explains nothing")
	}
	s, _ := content[0].(map[string]any)["text"].(string)
	return s
}
