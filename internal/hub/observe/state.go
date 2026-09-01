// package: hub/observe / state
// type:    logic (what a session says it is doing, as a value)
// job:     turn the coding tool's word for its own state into a TYPE, parsed once at the boundary,
// so nothing downstream can compare it to a literal it invented — and so a state added here is a
// compile error at every switch that has to account for it.
// limits:  the vocabulary and the two conversions. What a state MEANS for an agent's work is the
// orchestrator's (-> hub/situation.Surface).
package observe

// State is what a session reports about itself. Deliberately NOT a string type: a named string still
// compares equal to a bare literal, so the word would go on leaking into callers that should be
// asking. As an integer it cannot, and the words exist in exactly one place — the table below.
type State uint8

// The states a session can report. Unknown is the zero value and means the look failed or said
// something this vocabulary does not have — never "idle", which is a claim no failed look supports.
const (
	Unknown State = iota
	Working
	AtPrompt
	AwaitingHuman
	SignedOut
	TurnCutOff
)

// words is the coding tool's own vocabulary, and the ONLY place these strings appear. They are
// adapter/agent's State values, restated rather than imported: the orchestrator reads a state
// through this package, and importing that adapter is what the seam forbids it.
var words = map[State]string{
	Working:       "working",
	AtPrompt:      "idle",
	AwaitingHuman: "blocked",
	SignedOut:     "signed-out",
	TurnCutOff:    "api-error",
}

// ParseState reads the tool's word at the boundary, once. Anything unrecognised is Unknown, which is
// what a failed capture and a word from a newer tool have in common: no evidence either way.
func ParseState(word string) State {
	for s, w := range words {
		if w == word {
			return s
		}
	}
	return Unknown
}

// String is the canonical word, for the board and the front-end projections that render it. Unknown
// is "", the same empty the pane reports when a capture fails.
func (s State) String() string { return words[s] }
