package agent

import (
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
)

// TestPreviewSizeEnvOnlyWhenBothDimensionsSet: a launch with no preview (the CLI's every call)
// must leave the session at tmux's own default — sindri-agent.sh only sizes it when both env vars
// are present, so either being unset here must produce no entries at all.
func TestPreviewSizeEnvOnlyWhenBothDimensionsSet(t *testing.T) {
	cases := []struct {
		name        string
		cols, lines int
		want        map[string]string
	}{
		{"both unset", 0, 0, nil},
		{"cols only", 100, 0, nil},
		{"lines only", 0, 40, nil},
		{"negative", -1, 24, nil},
		{"both set", 132, 43, map[string]string{"SINDRI_COLS": "132", "SINDRI_LINES": "43"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := previewSizeEnv(c.cols, c.lines)
			if len(got) != len(c.want) {
				t.Fatalf("previewSizeEnv(%d, %d) = %v, want %v", c.cols, c.lines, got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("previewSizeEnv(%d, %d)[%q] = %q, want %q", c.cols, c.lines, k, got[k], v)
				}
			}
		})
	}
}

// TestModelEnvOnlyWhenChosen: no model chosen carries nothing — the account default, same as
// always — not an empty SINDRI_MODEL a relaunch would read as a real (if blank) value.
func TestModelEnvOnlyWhenChosen(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  map[string]string
	}{
		{"none chosen", "", nil},
		// Two values, and the id is the one that must stay whole: it is what --model is given, while
		// the label is only what the status bar shows (-> sindri-agent.sh).
		{"chosen", "claude-opus-5", map[string]string{
			"SINDRI_MODEL": "claude-opus-5", "SINDRI_MODEL_LABEL": "opus-5",
		}},
	}
	// The REAL backend, because the label is its knowledge: against the no-op the shortening is a
	// pass-through and this would assert nothing about it.
	agentport.Use(claude.New())
	t.Cleanup(func() { agentport.Use(unreadablePane{}) })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := modelEnv(c.model)
			if len(got) != len(c.want) {
				t.Fatalf("modelEnv(%q) = %v, want %v", c.model, got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("modelEnv(%q)[%q] = %q, want %q", c.model, k, got[k], v)
				}
			}
		})
	}
}
