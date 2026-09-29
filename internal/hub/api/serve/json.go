// package: hub/api/serve / json
// type:    adapter (HTTP request/response conventions)
// job:     the shapes and helpers every route shares — Decode a JSON body or answer 400,
// write a result or the hub's error, and flush a streamed body as it is produced.
// One place, so no handler invents its own error shape.
// limits:  no routing and no domain logic; which routes exist is server.go's.
package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// OKMsg is what a route that did something answers with — the verb in the past tense.
type OKMsg struct {
	OK string `json:"ok"`
}

// ErrMsg carries a refusal in the shape every front-end already reads.
type ErrMsg struct {
	Error string `json:"error"`
}

// Detached carries a request's context WITHOUT its cancellation, for work the hub must finish
// whatever the client then does: a pod coming up, a pod going away, a message landing. A read is the
// other case and takes r.Context() plain, so abandoning it abandons the work behind it.
func Detached(r *http.Request) context.Context { return context.WithoutCancel(r.Context()) }

// Decode reads a JSON body into v, answering 400 and reporting false when it cannot — so a route
// reads `if !Decode(...) { return }` and never repeats the error path.
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		WriteJSON(w, nil, err)
		return false
	}
	return true
}

// FlushWriter flushes after every write so streamed control-socket output (the
// launch / rebuild progress) reaches the client live rather than buffering.
type FlushWriter struct {
	w io.Writer
	f http.Flusher
}

// Flushing wraps w so streamed output reaches the client live, taking the flusher off it when it
// has one — which is the only thing a caller ever wanted from the two fields.
func Flushing(w io.Writer) *FlushWriter {
	fw := &FlushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.f = f
	}
	return fw
}

// Write passes the bytes on and flushes, so a long build reaches the client as it happens.
func (fw *FlushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}

// WriteJSON writes v as JSON, or a 400 with the error message if err != nil.
func WriteJSON(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrMsg{err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}
