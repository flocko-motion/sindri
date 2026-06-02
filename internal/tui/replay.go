// package: tui / replay
// type:    ui
// job:     scripted, in-process replay of the TUI Model for layout/hotkey
//          verification — parses a tiny key-sequence DSL into tea.Msg values,
//          drives Update + drains tea.Cmd to quiescence, and writes captured
//          View() frames (raw ANSI + stripped) on (capture <name>).
// limits:  no real TTY/subprocess; no live td/board (Fixture data only); time-
//          based cmds (ticks, blinks) are dropped via a per-cmd timeout.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/worker"
	"github.com/muesli/termenv"
)

// Fixture is the deterministic in-memory board the replay engine drives the
// Model against, so captures are reproducible across machines and runs.
type Fixture struct {
	Issues  []issue.Issue
	Workers []worker.Worker
	Width   int // initial terminal width  (default 100)
	Height  int // initial terminal height (default 30)

	// Descriptions / Comments are returned for the corresponding task IDs by
	// the test-mode fetchers, so the detail view doesn't have to shell out to
	// real td. A missing key returns the empty string (section omitted).
	Descriptions map[string]string
	Comments     map[string]string

	// LoadingState, when true, leaves the Model's loaded flag at false so the
	// startup "Loading…" placeholder can be captured. By default the engine
	// marks the model loaded once the fixture is applied.
	LoadingState bool
}

// Replay drives the TUI Model headlessly through a key-sequence script, writing
// captures to captureDir at every (capture <name>) directive. Each capture
// produces both <name>.ansi (raw, viewable with cat in a real terminal) and
// <name>.txt (stripped of ANSI for diff-friendly goldens). captureDir may be ""
// to skip writing.
func Replay(script string, fx Fixture, captureDir string) error {
	// Force truecolor so captures look like a real terminal session, regardless
	// of the host's TERM/NO_COLOR.
	lipgloss.SetColorProfile(termenv.TrueColor)

	// Swap the package-level fetchers so the detail view reads from the
	// fixture instead of shelling out to real td. Restore on the way out so
	// nothing leaks into other tests.
	prevDetail, prevComments := fetchTaskDetailFn, fetchTaskCommentsFn
	fetchTaskDetailFn = func(_, taskID string) string { return fx.Descriptions[taskID] }
	fetchTaskCommentsFn = func(_, taskID string) string { return fx.Comments[taskID] }
	defer func() {
		fetchTaskDetailFn = prevDetail
		fetchTaskCommentsFn = prevComments
	}()

	if captureDir != "" {
		if err := os.MkdirAll(captureDir, 0o755); err != nil {
			return err
		}
	}

	m := New(".")
	if fx.Width == 0 {
		fx.Width = 100
	}
	if fx.Height == 0 {
		fx.Height = 30
	}
	m.width, m.height = fx.Width, fx.Height
	m.resizeViewports()
	m.issues = append([]issue.Issue(nil), fx.Issues...)
	m.workers = append([]worker.Worker(nil), fx.Workers...)
	m.rebuildBacklog()
	if !fx.LoadingState {
		m.loaded = true // mirror the refreshMsg handler so existing goldens see "loaded" state
	}

	tokens, err := parseScript(script)
	if err != nil {
		return err
	}

	var model tea.Model = m
	for _, tok := range tokens {
		switch tok.kind {
		case tokKey:
			model = applyAndDrain(model, tok.key)
		case tokResize:
			model = applyAndDrain(model, tea.WindowSizeMsg{Width: tok.w, Height: tok.h})
		case tokDrain:
			model = applyAndDrain(model, nil)
		case tokCapture:
			if err := writeCapture(captureDir, tok.name, model.View()); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyAndDrain applies msg via Update, then runs returned tea.Cmds in
// goroutines with a short timeout so time-based ticks/blinks are dropped
// quickly, while quick action-result cmds get pumped back through Update. A
// nil msg only drains (used by the (drain) directive).
func applyAndDrain(model tea.Model, msg tea.Msg) tea.Model {
	var pending []tea.Cmd
	if msg != nil {
		next, cmd := model.Update(msg)
		model = next
		if cmd != nil {
			pending = append(pending, cmd)
		}
	}
	const cmdTimeout = 50 * time.Millisecond
	const maxIterations = 100
	for i := 0; len(pending) > 0 && i < maxIterations; i++ {
		cmd := pending[0]
		pending = pending[1:]

		out := runWithTimeout(cmd, cmdTimeout)
		if out == nil {
			continue // timed out (tick/blink) or returned nil; skip
		}
		// tea.BatchMsg fans out — pump each inner cmd
		if batch, ok := out.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					pending = append(pending, c)
				}
			}
			continue
		}
		next, cmd2 := model.Update(out)
		model = next
		if cmd2 != nil {
			pending = append(pending, cmd2)
		}
	}
	return model
}

// runWithTimeout executes cmd in a goroutine and returns its msg, or nil if it
// doesn't return within d. Used to skip tea.Tick / textinput.Blink etc.
func runWithTimeout(cmd tea.Cmd, d time.Duration) tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() {
		defer func() { recover() }() //nolint:errcheck // a cmd shouldn't panic, but if it does, skip it
		ch <- cmd()
	}()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(d):
		return nil
	}
}

// writeCapture writes <dir>/<name>.ansi (raw) and <dir>/<name>.txt (stripped).
func writeCapture(dir, name, view string) error {
	if dir == "" {
		return nil
	}
	if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(view), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".txt"), []byte(stripANSIRuntime(view)), 0o644)
}

// stripANSIRuntime drops CSI/SGR ANSI escapes for diff-friendly captures. The
// test-side stripANSI in list_view_test.go is the same shape but lives in test
// code; this is its production sibling.
func stripANSIRuntime(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 0x40 && r <= 0x7e) && r != '[' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// --- Script parsing -------------------------------------------------------

type tokKind int

const (
	tokKey tokKind = iota
	tokResize
	tokCapture
	tokDrain
)

type scriptToken struct {
	kind tokKind
	key  tea.KeyMsg
	w, h int
	name string
}

// specialKeys maps DSL names to a tea.KeyType (zero-Runes form).
var specialKeys = map[string]tea.KeyType{
	"down":      tea.KeyDown,
	"up":        tea.KeyUp,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEscape,
	"escape":    tea.KeyEscape,
	"tab":       tea.KeyTab,
	"space":     tea.KeySpace,
	"backspace": tea.KeyBackspace,
}

// ctrlKeys maps ctrl+<letter> suffixes to their tea.KeyType.
var ctrlKeys = map[byte]tea.KeyType{
	'a': tea.KeyCtrlA, 'b': tea.KeyCtrlB, 'c': tea.KeyCtrlC, 'd': tea.KeyCtrlD,
	'e': tea.KeyCtrlE, 'f': tea.KeyCtrlF, 'g': tea.KeyCtrlG, 'h': tea.KeyCtrlH,
	'i': tea.KeyCtrlI, 'j': tea.KeyCtrlJ, 'k': tea.KeyCtrlK, 'l': tea.KeyCtrlL,
	'm': tea.KeyCtrlM, 'n': tea.KeyCtrlN, 'o': tea.KeyCtrlO, 'p': tea.KeyCtrlP,
	'q': tea.KeyCtrlQ, 'r': tea.KeyCtrlR, 's': tea.KeyCtrlS, 't': tea.KeyCtrlT,
	'u': tea.KeyCtrlU, 'v': tea.KeyCtrlV, 'w': tea.KeyCtrlW, 'x': tea.KeyCtrlX,
	'y': tea.KeyCtrlY, 'z': tea.KeyCtrlZ,
}

// parseScript splits the script into whitespace-separated tokens, with
// parenthesized directives kept whole. Each non-directive token becomes one or
// more tea.KeyMsg (a recognised special key / ctrl-combo, otherwise one msg
// per literal rune — so "abc" types three keys).
func parseScript(script string) ([]scriptToken, error) {
	raw, err := tokenize(script)
	if err != nil {
		return nil, err
	}
	var out []scriptToken
	for _, t := range raw {
		if strings.HasPrefix(t, "(") {
			tok, err := parseDirective(t)
			if err != nil {
				return nil, err
			}
			out = append(out, tok)
			continue
		}
		// Key-ish token.
		if kt, ok := specialKeys[strings.ToLower(t)]; ok {
			out = append(out, scriptToken{kind: tokKey, key: tea.KeyMsg{Type: kt}})
			continue
		}
		if strings.HasPrefix(strings.ToLower(t), "ctrl+") && len(t) == 6 {
			letter := strings.ToLower(t)[5]
			if kt, ok := ctrlKeys[letter]; ok {
				out = append(out, scriptToken{kind: tokKey, key: tea.KeyMsg{Type: kt}})
				continue
			}
			return nil, fmt.Errorf("unknown ctrl-combo: %q", t)
		}
		// Literal runes: one keypress per character.
		for _, r := range t {
			out = append(out, scriptToken{kind: tokKey, key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}})
		}
	}
	return out, nil
}

// tokenize splits script into whitespace-separated tokens, treating
// parenthesized groups (e.g. "(capture list)") as a single token.
func tokenize(script string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inParen := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range script {
		switch {
		case r == '(':
			flush()
			cur.WriteRune(r)
			inParen = true
		case r == ')':
			if !inParen {
				return nil, fmt.Errorf("unmatched ')' in script")
			}
			cur.WriteRune(r)
			flush()
			inParen = false
		case inParen:
			cur.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if inParen {
		return nil, fmt.Errorf("unclosed '(' in script")
	}
	flush()
	return out, nil
}

// parseDirective parses a single "(name args...)" token.
func parseDirective(t string) (scriptToken, error) {
	body := strings.TrimSpace(t[1 : len(t)-1])
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return scriptToken{}, fmt.Errorf("empty directive: %q", t)
	}
	switch strings.ToLower(fields[0]) {
	case "capture":
		if len(fields) < 2 {
			return scriptToken{}, fmt.Errorf("%q: capture needs a name", t)
		}
		return scriptToken{kind: tokCapture, name: fields[1]}, nil
	case "resize":
		// Accept either "resize 120 40" or "resize 120x40".
		if len(fields) == 3 {
			w, h, err := parseInts(fields[1], fields[2])
			if err != nil {
				return scriptToken{}, fmt.Errorf("%q: %w", t, err)
			}
			return scriptToken{kind: tokResize, w: w, h: h}, nil
		}
		if len(fields) == 2 {
			parts := strings.SplitN(fields[1], "x", 2)
			if len(parts) != 2 {
				return scriptToken{}, fmt.Errorf("%q: resize wants W H or WxH", t)
			}
			w, h, err := parseInts(parts[0], parts[1])
			if err != nil {
				return scriptToken{}, fmt.Errorf("%q: %w", t, err)
			}
			return scriptToken{kind: tokResize, w: w, h: h}, nil
		}
		return scriptToken{}, fmt.Errorf("%q: resize wants W H or WxH", t)
	case "drain", "sleep":
		// (sleep N) is an alias for (drain): no wall-clock time is consumed.
		return scriptToken{kind: tokDrain}, nil
	default:
		return scriptToken{}, fmt.Errorf("unknown directive: %q", t)
	}
}

func parseInts(a, b string) (int, int, error) {
	var w, h int
	if _, err := fmt.Sscanf(a, "%d", &w); err != nil {
		return 0, 0, fmt.Errorf("bad width %q", a)
	}
	if _, err := fmt.Sscanf(b, "%d", &h); err != nil {
		return 0, 0, fmt.Errorf("bad height %q", b)
	}
	return w, h, nil
}
