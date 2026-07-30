package hub

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
