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

func TestContextUsageSumsTheLastAssistantUsage(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "sess", []string{
		`{"type":"user","message":{}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":5}}}`,
		`{"type":"user","message":{}}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":2,"cache_read_input_tokens":240480,"cache_creation_input_tokens":1129}}}`,
	})
	got, _, _, ok := Claude{}.ContextUsage(home)
	if !ok {
		t.Fatal("ContextUsage reported no usage, want the last assistant line's sum")
	}
	if want := 2 + 240480 + 1129; got != want {
		t.Errorf("ContextUsage = %d, want %d (the LAST assistant usage, not the first)", got, want)
	}
}

func TestContextUsageSkipsAssistantLinesWithNoUsage(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "sess", []string{
		`{"type":"assistant","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"assistant","message":{}}`, // a tool-use continuation line, no usage recorded
	})
	got, _, _, ok := Claude{}.ContextUsage(home)
	if !ok || got != 10 {
		t.Errorf("ContextUsage = (%d, %v), want (10, true) — should skip the trailing no-usage line", got, ok)
	}
}

func TestContextUsageNoSessionYet(t *testing.T) {
	home := t.TempDir()
	if _, _, _, ok := (Claude{}).ContextUsage(home); ok {
		t.Fatal("ContextUsage reported usage for a home with no transcript at all")
	}
}

func TestContextUsagePicksTheMostRecentSession(t *testing.T) {
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
	got, _, _, ok := Claude{}.ContextUsage(home)
	if !ok || got != 5 {
		t.Errorf("ContextUsage = (%d, %v), want the NEWER session's (5, true), not the older one's 999", got, ok)
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
	got, _, ok := lastUsage(path)
	if !ok || got != 7 {
		t.Errorf("lastUsage = (%d, %v), want (7, true) despite the oversized leading padding", got, ok)
	}
}

// TestTheWindowComesFromTheModel: a size means nothing alone — 480k is most of a 200k window and
// half a 1M one. The transcript names the model, so the window is read rather than assumed.
func TestTheWindowComesFromTheModel(t *testing.T) {
	for _, c := range []struct {
		model      string
		wantWindow int
	}{
		{"claude-opus-5", 1_000_000},
		{"claude-sonnet-5", 1_000_000},
		{"claude-haiku-4-5-20251001", 200_000},
		{"some-model-nobody-listed", defaultWindow}, // conservative: retire early, never never
	} {
		home := t.TempDir()
		writeTranscript(t, home, "sess", []string{
			`{"type":"assistant","message":{"model":"` + c.model + `","usage":{"input_tokens":42,` +
				`"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		})
		tokens, window, model, ok := Claude{}.ContextUsage(home)
		if !ok || tokens != 42 {
			t.Fatalf("%s: tokens = (%d, %v), want (42, true)", c.model, tokens, ok)
		}
		if window != c.wantWindow {
			t.Errorf("%s: window = %d, want %d", c.model, window, c.wantWindow)
		}
		if model != c.model {
			t.Errorf("model = %q, want %q", model, c.model)
		}
	}
}

// TestContextUsageReportsTheModel: the board shows what backs an agent, not just the window it
// resolves to — so the raw model id itself must come back too.
func TestContextUsageReportsTheModel(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "sess", []string{
		`{"type":"assistant","message":{"model":"claude-opus-5-20260315","usage":{"input_tokens":1,` +
			`"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
	})
	_, _, model, ok := Claude{}.ContextUsage(home)
	if !ok || model != "claude-opus-5-20260315" {
		t.Errorf("model = (%q, %v), want (%q, true)", model, ok, "claude-opus-5-20260315")
	}
}

// TestAnUnrecordedModelStillReportsAWindow: usage with no model must not report window 0, which the
// workflow reads as unknown and never retires on.
func TestAnUnrecordedModelStillReportsAWindow(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "sess", []string{
		`{"type":"assistant","message":{"usage":{"input_tokens":9,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
	})
	if _, window, _, ok := (Claude{}).ContextUsage(home); !ok || window != defaultWindow {
		t.Errorf("window = (%d, %v), want (%d, true)", window, ok, defaultWindow)
	}
}

// TestASyntheticTailStillResolvesTheWindow: live transcripts routinely end on a "<synthetic>" line
// Claude Code wrote itself. Sizing a 1M session against the fallback because of it would retire the
// agent at a fifth of its capacity, so the model comes from the newest line that names one.
func TestASyntheticTailStillResolvesTheWindow(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "sess", []string{
		`{"type":"assistant","message":{"model":"claude-opus-5","usage":{"input_tokens":2,` +
			`"cache_read_input_tokens":385000,"cache_creation_input_tokens":987}}}`,
		`{"type":"assistant","message":{"model":"<synthetic>","usage":{"input_tokens":0,` +
			`"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
	})
	tokens, window, model, ok := Claude{}.ContextUsage(home)
	if !ok {
		t.Fatal("ContextUsage reported nothing past the synthetic tail")
	}
	if want := 2 + 385000 + 987; tokens != want {
		t.Errorf("tokens = %d, want %d", tokens, want)
	}
	if window != 1_000_000 {
		t.Errorf("window = %d, want 1000000 — the synthetic line names no model, the one before it does", window)
	}
	if model != "claude-opus-5" {
		t.Errorf("model = %q, want %q — the synthetic line names no model, the one before it does", model, "claude-opus-5")
	}
}
