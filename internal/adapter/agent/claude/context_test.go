package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTranscript lays out home/projects/-workspace/<name>.jsonl with the given raw lines.
func writeTranscript(t *testing.T, home, name string, lines []string) string {
	t.Helper()
	dir := filepath.Join(home, "projects", "-workspace")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestContextTokensSumsTheLastAssistantUsage(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "sess", []string{
		`{"type":"user","message":{}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":5}}}`,
		`{"type":"user","message":{}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":2,"cache_read_input_tokens":240480,"cache_creation_input_tokens":1129}}}`,
	})
	got, ok := Claude{}.ContextTokens(home)
	if !ok {
		t.Fatal("ContextTokens reported no usage, want the last assistant line's sum")
	}
	if want := 2 + 240480 + 1129; got != want {
		t.Errorf("ContextTokens = %d, want %d (the LAST assistant usage, not the first)", got, want)
	}
}

func TestContextTokensSkipsAssistantLinesWithNoUsage(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "sess", []string{
		`{"type":"assistant","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"assistant","message":{}}`, // a tool-use continuation line, no usage recorded
	})
	got, ok := Claude{}.ContextTokens(home)
	if !ok || got != 10 {
		t.Errorf("ContextTokens = (%d, %v), want (10, true) — should skip the trailing no-usage line", got, ok)
	}
}

func TestContextTokensNoSessionYet(t *testing.T) {
	home := t.TempDir()
	if _, ok := (Claude{}).ContextTokens(home); ok {
		t.Fatal("ContextTokens reported usage for a home with no transcript at all")
	}
}

func TestContextTokensPicksTheMostRecentSession(t *testing.T) {
	home := t.TempDir()
	older := writeTranscript(t, home, "old", []string{
		`{"type":"assistant","message":{"usage":{"input_tokens":999,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
	})
	newer := writeTranscript(t, home, "new", []string{
		`{"type":"assistant","message":{"usage":{"input_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
	})
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	got, ok := Claude{}.ContextTokens(home)
	if !ok || got != 5 {
		t.Errorf("ContextTokens = (%d, %v), want the NEWER session's (5, true), not the older one's 999", got, ok)
	}
}

func TestLastUsageIgnoresATruncatedFirstLineFromTheTailSeek(t *testing.T) {
	// A real transcript can be much larger than tailBytes; readTail seeks into the middle of a
	// line. That partial first line must not be mistaken for real usage.
	dir := t.TempDir()
	path := filepath.Join(dir, "big.jsonl")
	padding := make([]byte, tailBytes+1024)
	for i := range padding {
		padding[i] = 'x'
	}
	body := string(padding) + "\n" + `{"type":"assistant","message":{"usage":{"input_tokens":7,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := lastUsage(path)
	if !ok || got != 7 {
		t.Errorf("lastUsage = (%d, %v), want (7, true) despite the oversized leading padding", got, ok)
	}
}
