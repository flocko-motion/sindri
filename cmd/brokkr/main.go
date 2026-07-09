// package: main (brokkr) / main
// type:    entrypoint
// job:     wires brokkr — sindri's toolbelt: the generic, hub-less developer
//          tools (`brokkr map`, `brokkr lint`) that work on any Go repo, with no
//          orchestration power. Named for Sindri's brother, who works the bellows
//          alongside the smith.
// limits:  no hub, no agents, no podman — just the tools; the linters live in
//          internal/lint and the map in internal/codemap.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// version is baked in at build time (-X main.version); "dev" for `go run`.
var version = "dev"

// versionLine is the single source of truth for brokkr's version string, so the
// bare command, --version, and the `version` subcommand all report identically.
// It includes the Go toolchain because a linter must run on current Go — a stale
// toolchain (or, just as easily, a stale brokkr shadowed on PATH) is a real cause
// of confusing results, and this is how you catch it.
func versionLine() string {
	return fmt.Sprintf("%s (built with %s)", version, runtime.Version())
}

// newVersionCmd wires `brokkr version` — the same string --version prints, but as a
// discoverable subcommand (people reach for `<tool> version` before `--version`,
// and checking it is the first step when brokkr behaves like an older build).
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print brokkr's version and the Go toolchain it was built with",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "brokkr %s\n", versionLine())
		},
	}
}

// --tail state, shared between the root's PersistentPreRunE (which decides whether
// to buffer and installs the buffer) and run() (which flushes it). tailBuf is
// non-nil only while --tail is active.
var (
	tailN   int
	tailBuf *bytes.Buffer
)

// exitCodeError lets a command request a specific process exit code after it has
// already printed its own explanation (e.g. lint's error/panic report), so main
// exits with that code without cobra echoing a redundant error or usage.
type exitCodeError struct{ code int }

func (e exitCodeError) Error() string { return "" }

func main() {
	root := &cobra.Command{
		Use:     "brokkr",
		Short:   "brokkr — sindri's toolbelt: code map + linters",
		Version: versionLine(),
		// Bare `brokkr` reports its own version and the Go toolchain it was built
		// with — a linter must run on current Go, so its toolchain is worth seeing
		// — then shows what it can do. Writes via the command's out writer so --tail
		// buffers it like everything else.
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "brokkr %s\n\n", versionLine())
			_ = cmd.Help()
		},
		// When --tail is set, redirect all command output into a buffer; run() prints
		// its tail and the exit marker once the command is done. Runs for whichever
		// subcommand executes (they define no hook of their own, so this root one is
		// used).
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if tailN > 0 {
				tailBuf = &bytes.Buffer{}
				cmd.Root().SetOut(tailBuf)
				cmd.Root().SetErr(tailBuf)
			}
			return nil
		},
	}
	root.PersistentFlags().IntVar(&tailN, "tail", 0,
		"buffer all output and print only its last N lines, then a final "+
			"'=== exit: <code> ===' line (0 = ran to completion; non-zero = an error or "+
			"panic stopped it early). Captures bounded output and the exit status in a "+
			"single command, so you don't need compound shell such as: "+
			"brokkr <cmd> 2>&1 | tail -N ; echo \"=== exit: $? ===\".")
	root.SilenceUsage = true // runtime errors report themselves; don't dump usage
	root.AddCommand(newMapCmd(), newLintCmd(), newVersionCmd())

	exit(run(root))
}

// exit is brokkr's ONLY process-exit path: it flushes the --tail buffer (its last
// N lines plus the "=== exit: <code>" marker) and then terminates with code. A raw
// os.Exit anywhere else would terminate before that flush and silently swallow the
// buffered output, so os.Exit is banned outside this function (enforced by
// TestNoRawOsExit). Commands must return their error/exit code up to run() rather
// than exiting themselves.
func exit(code int) {
	if tailBuf != nil {
		flushTail(os.Stdout, tailBuf.String(), tailN)
		fmt.Fprintf(os.Stdout, "=== exit: %d ===\n", code)
	}
	os.Exit(code)
}

// run executes the command tree and returns the process exit code, recovering a
// panic into a non-zero code (its stack goes to the tail buffer under --tail, so
// it survives into the printed tail; else to stderr). Subcommands signal failure
// by returning an error — an exitCodeError to pin a specific code — never os.Exit.
func run(root *cobra.Command) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(errSink(), "panic: %v\n\n%s\n", r, debug.Stack())
			code = 1
		}
	}()
	err := root.Execute()
	if err == nil {
		return 0
	}
	var ec exitCodeError
	if errors.As(err, &ec) {
		return ec.code
	}
	return 1 // a real error; cobra has already printed it to the (buffered) err writer
}

// errSink is where run() reports a recovered panic: the tail buffer when --tail is
// active (so it lands in the printed tail), else stderr.
func errSink() io.Writer {
	if tailBuf != nil {
		return tailBuf
	}
	return os.Stderr
}

// flushTail prints the last n lines of s to w (all of it when it has fewer), used
// to emit the buffered output under --tail.
func flushTail(w io.Writer, s string, n int) {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return
	}
	lines := strings.Split(s, "\n")
	if n < len(lines) {
		lines = lines[len(lines)-n:]
	}
	fmt.Fprintln(w, strings.Join(lines, "\n"))
}
