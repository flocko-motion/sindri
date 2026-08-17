// package: hub / http plumbing
// type:    adapter (HTTP request/response conventions)
// job:     the shapes and helpers every route shares — decode a JSON body or answer 400,
// write a result or the hub's error, and flush a streamed body as it is produced.
// One place, so no handler invents its own error shape.
// limits:  no routing and no domain logic; which routes exist is server.go's.
package hub

import (
	"encoding/json"
	"io"
	"net/http"
)

type okMsg struct {
	OK string `json:"ok"`
}

type errMsg struct {
	Error string `json:"error"`
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, nil, err)
		return false
	}
	return true
}

// flushWriter flushes after every write so streamed control-socket output (the
// launch / rebuild progress) reaches the client live rather than buffering.
type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}

// writeJSON writes v as JSON, or a 400 with the error message if err != nil.
func writeJSON(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errMsg{err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}
