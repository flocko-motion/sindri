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
}
