// package: hub/api/debugview / debugview
// type:    adapter (the flow debug view's loopback HTTP listener)
// job:     serve the flow debug page on 127.0.0.1, started on request and alive until the hub
// stops: the page, its vendored libraries, and exactly the four reads it draws from.
// limits:  READ-ONLY by construction — no route that acts is mounted, since any local process can
// reach a loopback port where only the socket's owner reaches the control socket. A request naming
// any other Host is refused, which is what stops a web page reaching it by DNS rebinding.
package debugview

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/serve"
)

//go:embed assets
var assets embed.FS

// Source is the four reads the page draws from.
type Source interface {
	// Graph is one flow's declarations; Subject one subject read against its flow, now.
	Graph(kind, variant string) (api.FlowGraph, error)
	Subject(kind, project, id string) (api.SubjectFlow, error)
	// State is the board, for the subject list; HandleEvents its change stream, the live trigger.
	State(selected string) (api.BoardState, error)
	HandleEvents(w http.ResponseWriter, r *http.Request)
}

// Server is the listener, idle until Serve is first called.
type Server struct {
	src Source

	mu   sync.Mutex
	srv  *http.Server
	port int
}

// New builds an idle server over src.
func New(src Source) *Server { return &Server{src: src} }

// Serve starts the listener on 127.0.0.1:port (0 lets the OS pick) and returns its URL. Already
// serving, it returns the running URL — unless port names a different one, which is refused.
func (s *Server) Serve(port int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		if port != 0 && port != s.port {
			return "", fmt.Errorf("the debug view is already served on port %d; it stays there until the hub restarts", s.port)
		}
		return s.url(), nil
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return "", fmt.Errorf("debug view: listen on 127.0.0.1:%d: %w", port, err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.srv = &http.Server{Handler: s.handler(s.port), ReadHeaderTimeout: 10 * time.Second}
	go func(srv *http.Server) {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("hub: debug view stopped serving: %v", err)
		}
	}(s.srv)
	return s.url(), nil
}

func (s *Server) url() string { return "http://127.0.0.1:" + strconv.Itoa(s.port) + "/" }

// Close stops the listener, if one was started.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		_ = s.srv.Close()
		s.srv = nil
	}
}

// handler is the whole surface: the page, its libraries and four GETs, behind the Host check.
func (s *Server) handler(port int) http.Handler {
	page, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err) // the embed is compiled in; a missing directory is a build that cannot run
	}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(page))
	mux.HandleFunc("GET /debug/graph", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		g, err := s.src.Graph(q.Get("kind"), q.Get("variant"))
		serve.WriteJSON(w, g, err)
	})
	mux.HandleFunc("GET /debug/subject", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, err := s.src.Subject(q.Get("kind"), q.Get("project"), q.Get("id"))
		serve.WriteJSON(w, v, err)
	})
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		st, err := s.src.State("")
		serve.WriteJSON(w, st, err)
	})
	mux.HandleFunc("GET /events", s.src.HandleEvents)
	return loopbackOnly(port, mux)
}

// loopbackOnly refuses a request whose Host is not this listener's own loopback address. A page on
// another origin can still make the browser send a request here; it cannot make it say this Host.
func loopbackOnly(port int, next http.Handler) http.Handler {
	p := strconv.Itoa(port)
	allowed := map[string]bool{"127.0.0.1:" + p: true, "localhost:" + p: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.Host] {
			http.Error(w, "the debug view answers only on its loopback address", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
