// package: ui/theme / glyph
// type:    rendering (shared presentation primitives)
// job:     the row markers both front-ends draw — one home for what each symbol is, so the CLI
// and the TUI cannot come to mean different things by the same mark, and a change is
// one file rather than a hunt through two.
// limits:  the symbols themselves; where a marker is placed, and the column it is padded to,
// belong to the front-end drawing it (which derives that width from these).
package theme

import "strconv"

// The pictorial markers are Nerd Font icons. A terminal without a patched font draws a box, which
// is accepted: the row still aligns, because the Private Use Area carries no East Asian Width for
// go-runewidth and the terminal to disagree over, and both count one cell whether the glyph exists
// or not — SAFER than the emoji they replaced, whose drawn width is what terminals argue about. Add
// only single-width icons: the Material Design range is drawn double and would undo that.
const (
	MarkAssigned = "\uf0ad" // nf-fa-wrench: a worker is on this task
	MarkDialIn   = "\uf06e" // nf-fa-eye: humans attached to an agent's session
	MarkWarning  = "\uf071" // nf-fa-warning: something is wrong and nobody has acted on it
	MarkRetired  = "\uf04d" // nf-fa-stop: an agent being wound down
	MarkMail     = "\uf0e0" // nf-fa-envelope: unread mail an agent has not picked up
)

// MarkClearArmed marks an agent with a context clear waiting for its next leaf boundary. It stays
// a plain symbol: it is legible in any font, so a patched one buys nothing here.
const MarkClearArmed = "␡"

// MarkNeedsUser marks what cannot move until the user acts. Plain ASCII deliberately: it sits
// inside the TUI's header strip and in a CLI line, and both read it as a word's worth of alarm
// rather than a picture.
const MarkNeedsUser = "!"

// The PR markers, a PAIR because the distinction is load-bearing: merging a final PR closes its
// task, while an interim one is a mid-task contribution that leaves the worker on it. The draft
// icon says which is which, where ◆ against ◇ said only "some difference".
const (
	MarkPRFinal   = "\uf407" // nf-oct-git_pull_request: the task's PR, whose merge ends it
	MarkPRInterim = "\uf4dd" // nf-oct-git_pull_request_draft: a contribution mid-task
)

// Superscripts are the tab hotkeys: "¹Tasks 12" cannot read the key as the count. A table, not an
// offset from '0' — ¹²³ are Latin-1 Supplement and the rest Superscripts and Subscripts.
var Superscripts = []string{
	"⁰", // ⁰ SUPERSCRIPT ZERO
	"¹", // ¹ SUPERSCRIPT ONE — Latin-1 Supplement, unlike its neighbours
	"²", // ² SUPERSCRIPT TWO — Latin-1 Supplement
	"³", // ³ SUPERSCRIPT THREE — Latin-1 Supplement
	"⁴", // ⁴ SUPERSCRIPT FOUR
	"⁵", // ⁵ SUPERSCRIPT FIVE
	"⁶", // ⁶ SUPERSCRIPT SIX
	"⁷", // ⁷ SUPERSCRIPT SEVEN
	"⁸", // ⁸ SUPERSCRIPT EIGHT
	"⁹", // ⁹ SUPERSCRIPT NINE
}

// Superscript renders n raised, falling back to plain digits past the table. A tenth tab has no
// hotkey anyway (onKey reads one character), so the fallback keeps it legible, not usable.
func Superscript(n int) string {
	if n < 0 || n >= len(Superscripts) {
		return strconv.Itoa(n)
	}
	return Superscripts[n]
}
