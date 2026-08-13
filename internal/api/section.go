// package: api / section
// type:    logic (a wire type)
// job:     one dashboard section as resolved for a client: a key, a title, and its
// badge count already computed against a board. hub/commands keeps the
// registry that computes Count (a func can't cross the wire); this is what
// crosses once the hub has run it.
// limits:  data only.
package api

// Section is one dashboard tab, resolved: the count is already read, not a
// recipe for reading it.
type Section struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Count int    `json:"count"`
	// Attention is how many of this section's rows are waiting on the USER — tasks nobody has ruled
	// on, agents that cannot move until a human acts (-> AgentNeedsUser). A subset of Count, and
	// the number every UI draws beside it as "(N!)". It rides on the section rather than being
	// worked out per tab, so a third marker is a line in the registry instead of a third special
	// case in each view.
	Attention int `json:"attention"`
}
