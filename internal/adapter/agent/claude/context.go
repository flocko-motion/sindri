// package: adapter/agent/claude / context
// type:    adapter (Claude Code — implements adapter/agent.Agent)
// job:     read a live session's current context size straight off Claude Code's own
// JSONL transcript — the last assistant message's usage IS the size of what
// it's carrying, exact unlike pane text (-> claude.go's DetectState).
// limits:  read-only. Several session files (relaunch, resume) are disambiguated
// by picking whichever was written to most recently.
package claude

import (
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tailBytes bounds how much of a transcript is read from its end — enough to survive a huge
// tool-result block between the last assistant turn and EOF, without reading a many-hundred-MB
// session whole on every probe.
const tailBytes = 4 << 20

// defaultWindow is assumed for an unrecognised model: the smallest any current Claude carries, so
// it retires early rather than never.
const defaultWindow = 200_000

// windows maps a model id fragment to its context window — the one place a window is stated. Any
// threshold hard-coded elsewhere is a guess about a model nobody checked.
var windows = []struct {
	match  string
	window int
}{
	{"opus-5", 1_000_000},
	{"sonnet-5", 1_000_000},
	{"haiku", 200_000},
}

// ModelWindow implements agent.Agent: model's window, ok=false when it matches nothing in the table
// above — a model the hub can start but whose window it cannot state is one whose fullness (and
// whose compaction threshold, a function of that window) it cannot judge, so callers choosing a
// model to launch on must refuse rather than fall back to a guess the way windowFor's sizing does.
func (Claude) ModelWindow(model string) (window int, ok bool) {
	for _, w := range windows {
		if strings.Contains(model, w.match) {
			return w.window, true
		}
	}
	return 0, false
}

// The compaction threshold falls as the window grows: pct(W) = P∞ + (P₀−P∞)·(W/W₀)^(−k). The same
// absolute overhead a 200k window pays in full is a smaller fraction of a bigger one, so the bar for
// compacting worth it falls with it. Named and kept beside the window table for the same reason that
// table gives itself: a threshold computed from a window guessed elsewhere is a guess nobody checked.
const (
	compactW0   = 200_000 // the window the curve is anchored to
	compactP0   = 0.375   // the fraction worth compacting at compactW0
	compactPInf = 0.05    // the floor the fraction falls toward as the window grows
	compactK    = 0.6     // how fast it falls between the two
)

// CompactionThreshold implements agent.Agent: the token count above which a session filling window
// tokens is worth summarizing rather than carrying — the curve above, in tokens rather than a bare
// fraction, since that is what a live reading is compared against.
func (Claude) CompactionThreshold(window int) int {
	if window <= 0 {
		return 0
	}
	pct := compactPInf + (compactP0-compactPInf)*math.Pow(float64(window)/compactW0, -compactK)
	return int(pct * float64(window))
}

// ContextUsage implements agent.Agent: what the session under home carries, the window it fills, and
// the raw model id carrying it. ok=false when nothing there has recorded usage yet.
func (Claude) ContextUsage(home string) (tokens, window int, model string, ok bool) {
	path, found := latestTranscript(home)
	if !found {
		return 0, 0, "", false
	}
	tokens, model, ok = lastUsage(path)
	if !ok {
		return 0, 0, "", false
	}
	return tokens, windowFor(model), model, true
}

// windowFor resolves a model id to its context window, conservatively when it is unrecognised.
func windowFor(model string) int {
	for _, w := range windows {
		if strings.Contains(model, w.match) {
			return w.window
		}
	}
	return defaultWindow
}

// latestTranscript finds home's most recently written *.jsonl under projects/*/ — Claude Code
// names the subdirectory after the container's cwd (always /workspace here), but the exact
// encoding is that tool's own business, so this globs rather than assumes the spelling.
func latestTranscript(home string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(home, "projects", "*", "*.jsonl"))
	if err != nil {
		return "", false
	}
	var best string
	var bestMod time.Time
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = m, info.ModTime()
		}
	}
	return best, best != ""
}

// transcriptLine is the subset of one JSONL record this package reads.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// syntheticModel marks a message Claude Code wrote itself — an interrupt or an error notice. It
// names no model, so it can say nothing about the window.
const syntheticModel = "<synthetic>"

// lastUsage scans path's tail backward for the size the session carries and the model carrying it,
// each from the newest line that can answer for it — not necessarily the same line, since a real
// transcript often ends on a synthetic one.
func lastUsage(path string) (tokens int, model string, ok bool) {
	tail, err := readTail(path, tailBytes)
	if err != nil {
		return 0, "", false
	}
	lines := strings.Split(string(tail), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var l transcriptLine
		if json.Unmarshal([]byte(line), &l) != nil || l.Type != "assistant" {
			continue // malformed, a truncated first line from the tail seek, or not an assistant turn
		}
		if model == "" && l.Message.Model != "" && l.Message.Model != syntheticModel {
			model = l.Message.Model
		}
		u := l.Message.Usage
		if !ok && (u.InputTokens != 0 || u.CacheReadInputTokens != 0 || u.CacheCreationInputTokens != 0) {
			tokens, ok = u.InputTokens+u.CacheReadInputTokens+u.CacheCreationInputTokens, true
		}
		if ok && model != "" {
			return tokens, model, true
		}
	}
	return tokens, model, ok
}

// readTail returns path's last max bytes (or the whole file, if smaller).
func readTail(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := int64(0)
	if info.Size() > max {
		start = info.Size() - max
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}
