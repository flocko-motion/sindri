// package: ui/theme / glyph
// type:    rendering (shared presentation primitives)
// job:     the row markers both front-ends draw — one home for what each symbol is, so the CLI
// and the TUI cannot come to mean different things by the same mark, and a change is
// one file rather than a hunt through two.
// limits:  the symbols themselves; where a marker is placed, and the column it is padded to,
// belong to the front-end drawing it (which derives that width from these).
package theme

// The pictorial markers are Nerd Font icons, taken from the Font Awesome block every Nerd Font has
// carried unchanged. A terminal without a patched font draws a box, which is accepted: the row
// still aligns, because these live in the Private Use Area, and PUA carries no East Asian Width for
// go-runewidth and the terminal to disagree over — both count one cell whether the glyph exists or
// not. That makes them SAFER than the emoji they replaced, whose drawn width is exactly what
// terminals argue about. Anything added here belongs to this block for the same reason: the
// Material Design range, notably, is drawn double-width.
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

// The PR markers: filled for a final PR, hollow for an interim one. They are a PAIR — the shapes
// carry the final-versus-interim distinction — so they only ever change together.
const (
	MarkPRFinal   = "◆"
	MarkPRInterim = "◇"
)
