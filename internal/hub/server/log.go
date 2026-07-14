// package: hub/server / log
// type:    logic (HTTP access logging)
// job:     the access-log middleware for the hub's sockets — wrap a handler so every
//          request records one coalesced access-log line (see accesslog.go), skipping
//          the long-lived SSE streams. The single access log is shared by the control
//          socket and every agent socket, so it lives here, not with the routes.
// limits:  transport plumbing only; what each route does is the hub's (routes).
package server

import (
	"net/http"
	"os"
	"time"

	"golang.org/x/term"
)

// streamingPaths are long-lived by design (SSE): they'd hold an access-log run open
// for the life of the connection, so they're left out of the log entirely.
var streamingPaths = map[string]bool{"/events": true, "/chat/stream": true}

// accessLogger coalesces the access log so high-frequency UI polling collapses to
// counted lines instead of flooding. It writes to os.Stderr — where log(1) and the
// background hub's redirected hub.log both go — and rewrites lines in place when that
// is a terminal (foreground hub).
var accessLogger = newAccessLog(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))

// LogRequests wraps a handler to record one access-log entry per request — the hub's
// window onto every action it executes. label is the socket's owner ("hub" or an
// agent name). Entries are coalesced, so a repeated read shows as one counted line.
func LogRequests(label string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if streamingPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		accessLogger.record(label, r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// FlushAccessLog ends any open access-log run — called when the hub stops so a
// trailing burst isn't lost.
func FlushAccessLog() { accessLogger.Flush() }

// statusRecorder captures the response status while passing flushing through (needed
// for the streamed /exec and SSE endpoints).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
