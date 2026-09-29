package debugview

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// fakeSource answers the four reads with fixed shapes.
type fakeSource struct{}

func (fakeSource) Graph(kind, variant string) (api.FlowGraph, error) {
	return api.FlowGraph{Kind: kind, Variant: variant}, nil
}
func (fakeSource) Subject(kind, project, id string) (api.SubjectFlow, error) {
	return api.SubjectFlow{Kind: kind, Project: project, ID: id}, nil
}
func (fakeSource) State(string) (api.BoardState, error) { return api.BoardState{}, nil }
func (fakeSource) HandleEvents(w http.ResponseWriter, _ *http.Request) {
	_, _ = io.WriteString(w, "data: {}\n\n")
}

// freePort is a port nothing holds this moment, so a test never depends on DefaultPort being free.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func served(t *testing.T) (*Server, string) {
	t.Helper()
	s := New(fakeSource{})
	url, err := s.startFrom(freePort(t), 5)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, url
}

func do(t *testing.T, method, url, host string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// TestTheViewServesThePageAndItsReads: the page, its vendored libraries and the four reads.
func TestTheViewServesThePageAndItsReads(t *testing.T) {
	_, url := served(t)
	for _, path := range []string{"", "favicon.png", "vendor/cytoscape.min.js", "vendor/dagre.min.js", "vendor/cytoscape-dagre.js",
		"debug/graph?kind=agent&variant=worker", "debug/subject?kind=agent&project=p&id=a", "state", "events"} {
		if resp := do(t, "GET", url+path, ""); resp.StatusCode != http.StatusOK {
			t.Errorf("GET /%s = %d, want 200", path, resp.StatusCode)
		}
	}
	body, _ := io.ReadAll(do(t, "GET", url, "").Body)
	if !strings.Contains(string(body), "cytoscape") {
		t.Error("the page does not load cytoscape")
	}
}

// TestTheViewActsOnNothing: any local process can reach a loopback port, so nothing that writes may be
// reachable — a write verb is refused, and the socket's command routes are simply not there.
func TestTheViewActsOnNothing(t *testing.T) {
	_, url := served(t)
	if resp := do(t, "POST", url+"debug/graph", ""); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", resp.StatusCode)
	}
	for _, path := range []string{"launch", "agent/stop", "merge", "debug/serve"} {
		if resp := do(t, "GET", url+path, ""); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET /%s = %d, want 404 — no command route is mounted here", path, resp.StatusCode)
		}
	}
}

// TestAnotherHostIsRefused is the DNS-rebinding guard: a page elsewhere can aim the browser at this
// port under its own name, and that name is what the check refuses.
func TestAnotherHostIsRefused(t *testing.T) {
	_, url := served(t)
	if resp := do(t, "GET", url+"state", "evil.example:80"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a foreign Host got %d, want 403", resp.StatusCode)
	}
	port := strings.TrimSuffix(strings.TrimPrefix(url, "http://127.0.0.1:"), "/")
	if resp := do(t, "GET", url+"state", "localhost:"+port); resp.StatusCode != http.StatusOK {
		t.Errorf("localhost got %d, want 200", resp.StatusCode)
	}
}

// TestTheViewTakesTheFirstFreePortFromItsOwn: the URL stays the same across restarts, so the view
// binds its first port whenever it can, and the next one up only when that one is held.
func TestTheViewTakesTheFirstFreePortFromItsOwn(t *testing.T) {
	first := freePort(t)
	held, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first)))
	if err != nil {
		t.Skipf("could not hold port %d for the test: %v", first, err)
	}
	defer held.Close()
	s := New(fakeSource{})
	t.Cleanup(s.Close)
	url, err := s.startFrom(first, 5)
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://127.0.0.1:" + strconv.Itoa(first+1) + "/"; url != want {
		t.Errorf("with %d held the view is at %s, want the next port up, %s", first, url, want)
	}
	if s.Where() != url {
		t.Errorf("Where = %q, want the served URL %q", s.Where(), url)
	}
}

// TestNoFreePortIsSaidNotSwallowed: a view that could not bind says why, where hub info reads it.
func TestNoFreePortIsSaidNotSwallowed(t *testing.T) {
	first := freePort(t)
	held, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first)))
	if err != nil {
		t.Skipf("could not hold port %d for the test: %v", first, err)
	}
	defer held.Close()
	s := New(fakeSource{})
	if _, err := s.startFrom(first, 1); err == nil {
		t.Fatal("the only port was held, yet the view claims to serve")
	}
	if w := s.Where(); !strings.HasPrefix(w, "not served: no free port") {
		t.Errorf("Where = %q, want the reason it is not served", w)
	}
}
