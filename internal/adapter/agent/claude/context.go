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
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tailBytes bounds how much of a transcript is read from its end — enough to survive a huge
// tool-result block between the last assistant turn and EOF, without reading a many-hundred-MB
// session whole on every probe.
const tailBytes = 4 << 20

// ContextTokens implements agent.Agent: the current context size of the session under home, or
// ok=false when no session there has recorded usage yet.
func (Claude) ContextTokens(home string) (int, bool) {
	path, ok := latestTranscript(home)
	if !ok {
		return 0, false
	}
	return lastUsage(path)
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
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// lastUsage scans path's tail backward for the last assistant message carrying usage and sums the
// three fields that make up its context size.
func lastUsage(path string) (int, bool) {
	tail, err := readTail(path, tailBytes)
	if err != nil {
		return 0, false
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
		u := l.Message.Usage
		if u.InputTokens == 0 && u.CacheReadInputTokens == 0 && u.CacheCreationInputTokens == 0 {
			continue // no usage on this line — keep scanning backward for one that has it
		}
		return u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens, true
	}
	return 0, false
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
