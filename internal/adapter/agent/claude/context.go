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

// tailBytes bounds how much of a transcript is read from its end, past a huge tool-result block,
// without reading a many-hundred-MB session whole on every probe.
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

// modelPrefix is the vendor prefix every Claude model id carries. It therefore distinguishes nothing,
// in a column sitting beside the context fill and the agent name.
const modelPrefix = "claude-"

// ShortModel implements agent.Agent: the model as a column wants it, the vendor prefix gone. DISPLAY
// ONLY — the full id is what ModelWindow matches on and what the dispatcher compares, so it is
// shortened where it is rendered and nowhere earlier. An id WITHOUT the prefix comes back unchanged:
// "I do not know this model" must not render as "this agent has no model".
func (Claude) ShortModel(model string) string {
	return strings.TrimPrefix(model, modelPrefix)
}

// ModelWindow implements agent.Agent: model's window, ok=false when it matches nothing in the table
// above — refuse rather than guess, since a window this can't state is fullness it can't judge.
func (Claude) ModelWindow(model string) (window int, ok bool) {
	for _, w := range windows {
		if strings.Contains(model, w.match) {
			return w.window, true
		}
	}
	return 0, false
}

// tierModels maps each difficulty tier to the model it dispatches to — the one place a tier
// resolves to an actual model id, so a caller never invents its own mapping.
var tierModels = map[string]string{
	"junior": "claude-haiku-4-5",
	"mid":    "claude-sonnet-5",
	"senior": "claude-opus-5",
}

// ModelForTier implements agent.Agent: the model a task of this tier dispatches to, ok=false for
// anything outside api.TierWords — refuse rather than guess, same rule as ModelWindow.
func (Claude) ModelForTier(tier string) (model string, ok bool) {
	m, found := tierModels[tier]
	return m, found
}

// ModelMatches implements agent.Agent: is detected want's family, not want verbatim — Haiku 4.5's
// real id carries a dated snapshot suffix (claude-haiku-4-5-20251001) the plain tier id never names.
func (Claude) ModelMatches(want, detected string) bool {
	return strings.Contains(detected, want)
}

// The compaction threshold falls as the window grows: pct(W) = P∞ + (P₀−P∞)·(W/W₀)^(−k). The same
// absolute overhead is a smaller fraction of a bigger window, so the bar for compacting falls with it.
const (
	compactW0   = 200_000 // the window the curve is anchored to
	compactP0   = 0.375   // the fraction worth compacting at compactW0
	compactPInf = 0.05    // the floor the fraction falls toward as the window grows
	compactK    = 0.863   // how fast it falls between the two — fit to the epic's own table (sd-43fa4a):
	// every anchor past the 200k one it's pinned at (500k/1M/2M/4M/8M) only reproduces near this k,
	// not the 0.6 first written down; 200k fits any k since (W/W0)^-k is 1 there regardless.
)

// CompactionThreshold implements agent.Agent: the curve above, in tokens rather than a bare
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

// latestTranscript finds home's most recently written *.jsonl under projects/*/ — globbed rather
// than assumed, since the exact subdirectory spelling is Claude Code's own business.
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

// lastUsage scans path's tail backward for the size and model, each from the newest line that can
// answer for it — not necessarily the same line, since a real transcript often ends on a synthetic one.
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
