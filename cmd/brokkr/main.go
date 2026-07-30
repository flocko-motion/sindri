// package: main (brokkr) / main
// type:    entrypoint
// job:     wires brokkr — sindri's toolbelt: the generic, hub-less developer
// tools (`brokkr map`, `brokkr lint`) that work on any Go repo, with no
// orchestration power. Named for Sindri's brother, who works the bellows
// alongside the smith.
// limits:  no hub, no agents, no podman — just the tools; the linters live in
// internal/lint and the map in internal/codemap.
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

// buildTime is when the binary was LINKED (-X main.buildTime); Go records only the COMMIT time.
// Kept out of version, which the hub compares — a timestamp would always look new.
var buildTime = ""

// versionLine heads the help output, so it stays one line; `brokkr version` prints versionDetail.
func versionLine() string {
	return fmt.Sprintf("%s (built with %s)", version, runtime.Version())
}

// versionDetail answers "which build is this, exactly?". A field the toolchain never recorded is
// printed as missing with the reason, never omitted.
func versionDetail() string {
	var b strings.Builder
	fmt.Fprintf(&b, "brokkr %s\n", version)
	row := func(label, value string) { fmt.Fprintf(&b, "  %-11s %s\n", label, value) }
	row("go", runtime.Version())
	row("platform", runtime.GOOS+"/"+runtime.GOARCH)
	if buildTime != "" {
		row("built", buildTime)
	} else {
		row("built", "not stamped (`go run`, or a build that bypassed the Makefile)")
	}
	rev, when, dirty, ok := vcsInfo()
	if !ok {
		row("commit", "not recorded (built without VCS stamping)")
		return b.String()
	}
	if dirty {
		rev += " (uncommitted changes at build time)"
	}
	row("commit", rev)
	if when != "" {
		row("commit time", when)
	}
	return b.String()
}

// vcsInfo reads Go's embedded git stamp; ok is false when absent (`go run`, -buildvcs=false).
func vcsInfo() (rev, when string, dirty, ok bool) {
	info, avail := debug.ReadBuildInfo()
	if !avail {
		return "", "", false, false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			when = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return rev, when, dirty, rev != ""
}

// newVersionCmd wires `brokkr version`, since a hand reaches for that before `--version`.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print brokkr's build identity: version, Go toolchain, platform, commit, build time",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprint(cmd.OutOrStdout(), versionDetail())
		},
	}
}

// --tail state, shared between PersistentPreRunE (installs the buffer) and run() (flushes it).
var (
	tailN   int
	tailBuf *bytes.Buffer
)

// exitCodeError pins an exit code for a command that already explained itself, so cobra stays quiet.
type exitCodeError struct{ code int }

func (e exitCodeError) Error() string { return "" }

func main() {
	root := &cobra.Command{
		Use:   "brokkr",
		Short: "brokkr — sindri's toolbelt: code map + linters",
		Long: "brokkr — sindri's toolbelt: code map + linters.\n\n" +
			"Built for AI coding agents: one self-contained command per feature. You should never " +
			"need a compound command — no pipes, no `&&`, no grepping or awk-ing what it prints. " +
			"The urge to reach for one is a SYMPTOM of a missing feature, not a problem to route " +
			"around: say so, and it gets fixed.",
		Version: versionLine(),
		// Bare `brokkr` states its build, then what it can do. Via the command's writer, so
		// --tail buffers it like everything else.
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "brokkr %s\n\n", versionLine())
			_ = cmd.Help()
		},
		// Under --tail, buffer output; run() prints the tail. This root hook covers every
		// subcommand, since none defines its own.
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

// exit is brokkr's ONLY process-exit path — it flushes the --tail buffer first. A raw os.Exit
// elsewhere would swallow that output, so it is banned (TestNoRawOsExit).
func exit(code int) {
	if tailBuf != nil {
		flushTail(os.Stdout, tailBuf.String(), tailN)
		fmt.Fprintf(os.Stdout, "=== exit: %d ===\n", code)
	}
	os.Exit(code)
}

// run executes the command tree and returns the exit code, recovering a panic into a non-zero one
// (its stack goes to the tail buffer, so it survives). Failure travels as an error, never os.Exit.
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

// errSink puts a recovered panic in the tail buffer when --tail is active, else on stderr.
func errSink() io.Writer {
	if tailBuf != nil {
		return tailBuf
	}
	return os.Stderr
}

// flushTail prints the last n lines of s (all of it when fewer) — the --tail buffer's output.
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
