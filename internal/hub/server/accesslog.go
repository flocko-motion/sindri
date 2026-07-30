// package: hub/server / accesslog
// type:    logic (access-log coalescing)
// job:     keep the hub's access log from flooding: consecutive identical entries
// (same label+method+path) collapse into one counted line instead of
// being dropped, so high-frequency UI polling stays visible but compact.
// limits:  formatting + coalescing only; the HTTP wrapper (logRequests) feeds it.
package server

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// accessLog coalesces a run of identical entries (same label+method+path) into one line. Rendering
// differs by sink because only one can rewrite: a terminal updates the line in place, while the
// append-only log prints the first hit at once and appends a "(×N)" summary when the run ends.
type accessLog struct {
	mu  sync.Mutex
	out io.Writer
	tty bool
	now func() time.Time // injectable for tests

	key   string // current run's identity, "" when no run is open
	body  string // rendered body of the latest entry in the run (no ts/count/newline)
	count int
	dirty bool // terminal: an un-terminated line is currently on screen
}

func newAccessLog(out io.Writer, tty bool) *accessLog {
	return &accessLog{out: out, tty: tty, now: time.Now}
}

const accessLogTime = "2006/01/02 15:04:05"

// record logs one request, coalescing it with an identical immediate predecessor.
func (a *accessLog) record(label, method, path string, status int, dur time.Duration) {
	key := label + "\t" + method + "\t" + path
	body := fmt.Sprintf("%-8s %-4s %-14s %d %s", label, method, path, status, dur.Round(time.Millisecond))

	a.mu.Lock()
	defer a.mu.Unlock()
	ts := a.now().Format(accessLogTime)

	if key == a.key { // same endpoint again → extend the run
		a.count++
		a.body = body
		if a.tty {
			// Rewrite the visible line; clear to EOL so a shorter render leaves no
			// trailing chars from the previous, longer one.
			fmt.Fprintf(a.out, "\r%s %s  (×%d)\033[K", ts, body, a.count)
		}
		return
	}

	a.finalizeLocked() // close out the previous run before starting a new one
	a.key, a.body, a.count = key, body, 1
	if a.tty {
		fmt.Fprintf(a.out, "%s %s", ts, body) // no newline: may be rewritten in place
		a.dirty = true
	} else {
		fmt.Fprintf(a.out, "%s %s\n", ts, body) // first hit shown immediately
	}
}

// finalizeLocked closes the current run: on a terminal it terminates the live
// line; in a file it appends a repeat summary when the run had more than one hit.
// Caller holds a.mu.
func (a *accessLog) finalizeLocked() {
	if a.key == "" {
		return
	}
	switch {
	case a.tty:
		if a.dirty {
			fmt.Fprint(a.out, "\n")
			a.dirty = false
		}
	case a.count > 1:
		// The first hit was already printed; summarize the whole burst by total.
		fmt.Fprintf(a.out, "%s %s  (×%d)\n", a.now().Format(accessLogTime), a.body, a.count)
	}
	a.key, a.count = "", 0
}

// Flush ends any open run, so a trailing burst isn't lost when the hub stops.
func (a *accessLog) Flush() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.finalizeLocked()
}
