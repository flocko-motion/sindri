package hub

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The file-sink access log shows every distinct request once and collapses a
// repeated endpoint into a single "(×N)" summary instead of dropping or flooding.
func TestAccessLogCoalesceFile(t *testing.T) {
	var buf bytes.Buffer
	a := newAccessLog(&buf, false) // false => file sink (no in-place rewrite)
	a.now = func() time.Time { return time.Unix(0, 0).UTC() }

	a.record("hub", "GET", "/agent/pane", 200, 0)
	a.record("hub", "GET", "/agent/pane", 200, 0)
	a.record("hub", "GET", "/agent/pane", 200, 0) // run of 3
	a.record("hub", "GET", "/tasks", 200, 0)      // different entry ends the run
	a.Flush()                                     // close the trailing /tasks run

	out := buf.String()
	// The repeated endpoint is printed once up front, then summarized by total.
	if n := strings.Count(out, "/agent/pane"); n != 2 {
		t.Errorf("/agent/pane should appear twice (first hit + summary), got %d in:\n%s", n, out)
	}
	if !strings.Contains(out, "(×3)") {
		t.Errorf("burst of 3 should summarize as (×3); got:\n%s", out)
	}
	// A distinct, non-repeated request is shown plainly with no counter.
	if !strings.Contains(out, "/tasks") || strings.Contains(out, "/tasks 200 0s  (×") {
		t.Errorf("single /tasks should log once without a counter; got:\n%s", out)
	}
}

// The terminal-sink access log rewrites the run's line in place (\r) as repeats
// arrive, rather than emitting a new line each time.
func TestAccessLogCoalesceTTY(t *testing.T) {
	var buf bytes.Buffer
	a := newAccessLog(&buf, true) // true => terminal sink
	a.now = func() time.Time { return time.Unix(0, 0).UTC() }

	a.record("hub", "GET", "/state", 200, 0)
	a.record("hub", "GET", "/state", 200, 0)

	out := buf.String()
	if !strings.Contains(out, "\r") || !strings.Contains(out, "(×2)") {
		t.Errorf("repeat on a terminal should rewrite the line in place with a count; got %q", out)
	}
	if strings.Count(out, "\n") != 0 {
		t.Errorf("an open run should not emit a newline until it ends; got %q", out)
	}
}

// logRequests never records the SSE stream (it would hold a run open for the
// life of the connection).
func TestLogRequestsSkipsStream(t *testing.T) {
	orig := accessLogger
	var buf bytes.Buffer
	accessLogger = newAccessLog(&buf, false)
	defer func() { accessLogger = orig }()

	h := logRequests("hub", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/events", nil))
	if buf.Len() != 0 {
		t.Errorf("/events stream should never be logged; got %q", buf.String())
	}
}

// requireProject rejects a repo-scoped request with no X-Sindri-Project header, but
// lets the global board reads (and any request carrying the header) through.
func TestRequireProject(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := requireProject(next)

	code := func(method, path, project string) int {
		req := httptest.NewRequest(method, path, nil)
		if project != "" {
			req.Header.Set("X-Sindri-Project", project)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := code("POST", "/launch", ""); got != http.StatusBadRequest {
		t.Errorf("repo-scoped /launch without project = %d, want 400", got)
	}
	if got := code("POST", "/launch", "/repo"); got != http.StatusOK {
		t.Errorf("/launch with project = %d, want 200", got)
	}
	if got := code("GET", "/state", ""); got != http.StatusOK {
		t.Errorf("global /state without project = %d, want 200", got)
	}
	if got := code("GET", "/events", ""); got != http.StatusOK {
		t.Errorf("global /events without project = %d, want 200", got)
	}
}
