// package: debug / debug
// type:    logic (process-wide debug logging switch)
// job:     one global verbosity toggle any package can honor without importing the
//          CLI — the `--debug` flag flips it on, and Logf emits to stderr only when
//          it's set. So a puzzling failure (a 403, a wedged probe) can be made to
//          explain itself without threading a logger through every call.
// limits:  a boolean + a formatter; no levels, no files, no config of its own.
package debug

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
)

// on is the process-wide switch (atomic so a background goroutine can read it while
// the CLI sets it during startup).
var on atomic.Bool

// out is where debug lines go (stderr by default; overridable for tests).
var out io.Writer = os.Stderr

// SetEnabled turns debug logging on or off — called once from the CLI when it parses
// the global --debug flag.
func SetEnabled(b bool) { on.Store(b) }

// Logf writes a "[debug] …" line to stderr when debug is enabled; a no-op otherwise.
func Logf(format string, args ...any) {
	if on.Load() {
		fmt.Fprintf(out, "[debug] "+format+"\n", args...)
	}
}

// SetOutput redirects debug output (used by tests to capture it).
func SetOutput(w io.Writer) { out = w }
