// package: api / flowgraph
// type:    logic (wire types: a declared flow, and one subject's place in it)
// job:     the shapes the flow debug view reads — a flow's states and every edge out of each, and
// where one subject stands in it, which of those edges hold now, and how it got there.
// limits:  shapes. Building them is the machine's and each subject's engine's (-> machine.Graph).
package api

// FlowGraph is one declared flow. Kind names the machine ("agent"); Variant the flow within it
// (a role, for an agent).
type FlowGraph struct {
	Kind    string      `json:"kind"`
	Variant string      `json:"variant"`
	Start   string      `json:"start"`
	States  []FlowState `json:"states"`
	Groups  []FlowGroup `json:"groups,omitempty"`
	// Positions is the checked-in default drawing, by state name; LayoutFile and LayoutPackage say
	// where a developer's export of it belongs (repo-relative path, Go package name).
	Positions     map[string]FlowPos `json:"positions,omitempty"`
	LayoutFile    string             `json:"layoutFile,omitempty"`
	LayoutPackage string             `json:"layoutPackage,omitempty"`
}

// FlowPos is one state's default position in the drawing.
type FlowPos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// FlowGroup is a parent state: a region whose exits every state inside it inherits. First exits are
// checked before a child's own, Then after.
type FlowGroup struct {
	Name  string      `json:"name"`
	Title string      `json:"title"`
	About string      `json:"about"`
	In    string      `json:"in,omitempty"`
	First []FlowEvent `json:"first,omitempty"`
	Then  []FlowEvent `json:"then,omitempty"`
}

// FlowExitRef names one of a state's effective exits: whose list it is in (the state's own, or a
// group's First or Then) and where. A state's Exits run in the order a pass checks them, which is the
// order SubjectFlow.Holds follows.
type FlowExitRef struct {
	Owner string `json:"owner"`
	First bool   `json:"first,omitempty"`
	Index int    `json:"index"`
}

// FlowState is one declared state. Action is "" where the hub does nothing; In is the group it sits
// in; Events its own exits; Exits every exit it has, inherited included, in checking order.
type FlowState struct {
	Name     string        `json:"name"`
	Title    string        `json:"title"`
	About    string        `json:"about"`
	In       string        `json:"in,omitempty"`
	Exits    []FlowExitRef `json:"exits"`
	Action   string        `json:"action,omitempty"`
	Outcomes []string      `json:"outcomes,omitempty"`
	Awaits   bool          `json:"awaits,omitempty"`
	WhenIdle string        `json:"whenIdle"` // "rest", "nudge" or "undeclared"
	Says     string        `json:"says,omitempty"`
	Tells    bool          `json:"tells,omitempty"`
	Events   []FlowEvent   `json:"events"`
	Verbs    []FlowVerb    `json:"verbs,omitempty"`
}

// FlowEvent is one edge out of a state, in declaration order: the first that holds is the one
// taken. To is "" for an event that moves nobody. Within and Wake are a condition's. Kind is what the
// exit is in the flow (-> machine.Kind); Trigger what fires it.
type FlowEvent struct {
	On      string   `json:"on"`
	Kind    string   `json:"kind"`    // progress, setback, fault, intervention, upkeep, world-moved; "" untagged
	Trigger string   `json:"trigger"` // "outcome", "condition" or "orphaned"
	To      string   `json:"to"`
	Why     string   `json:"why"`
	Within  string   `json:"within,omitempty"`
	Wake    []string `json:"wake,omitempty"`
}

// FlowVerb is a verb a state offers, and why.
type FlowVerb struct {
	Verb string `json:"verb"`
	Why  string `json:"why"`
}

// SubjectFlow is one subject's place in its flow: the state it stands in, where a pass would settle
// it, whether each of that state's effective exits holds now (FlowState.Exits' order), the facts those
// were read from, and its history, newest first.
type SubjectFlow struct {
	Kind    string        `json:"kind"`
	Project string        `json:"project"`
	ID      string        `json:"id"`
	Variant string        `json:"variant"`
	State   string        `json:"state"`
	Would   string        `json:"would"`
	Holds   []bool        `json:"holds"`
	Facts   []FlowFact    `json:"facts"`
	History []FlowHistory `json:"history"`
}

// FlowFact is one named reading behind a subject's conditions — for an agent, the observer's.
type FlowFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FlowHistory is one state-log row, with the edge it took when it records a move.
type FlowHistory struct {
	StateEvent
	Move *FlowMove `json:"move,omitempty"`
}

// FlowMove is the edge a recorded move took.
type FlowMove struct {
	From string `json:"from"`
	On   string `json:"on"`
	To   string `json:"to"`
}
