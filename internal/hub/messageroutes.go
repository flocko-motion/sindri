// package: hub / message routes
// type:    adapter (HTTP transport)
// job:     the routes that carry a message to an agent or read one back — PUSH it into the
// live session, or fetch a message from its mailbox in full. Grouped because they are
// the two halves of one question (must it be read, must it wake) rather than two
// unrelated endpoints.
// limits:  pure transport over Hub methods; the classification is the sender's
// (-> workflow.Delivery) and the mailbox is the store's.
package hub

import (
	"fmt"
	"net/http"
	"strconv"
)

// messageRoutes registers the message endpoints on mux.
func (h *Hub) messageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /tell", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"delivered"}, h.agents.Tell(detached(r), h.agentReq(r, req.Name), req.Name, req.Msg, req.Source, req.SignedOut))
	})
	// The other half of the pair: mail waits to be read and does not interrupt, where /tell wakes the
	// agent now and is lost if it is not there. Two routes, because the choice is the user's.
	mux.HandleFunc("POST /agent/mail", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, okMsg{"mailed"}, h.MailAgent(h.agentReq(r, req.Name), req.Name, req.Msg))
	})
	// The user answering an agent's message, from either front-end: the recipient comes from the row.
	mux.HandleFunc("POST /mail/reply", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq // Name carries the message id, Msg the reply
		if !decode(w, r, &req) {
			return
		}
		id, err := strconv.ParseInt(req.Name, 10, 64)
		if err != nil {
			writeJSON(w, nil, fmt.Errorf("mail id must be a number, got %q", req.Name))
			return
		}
		writeJSON(w, okMsg{"replied"}, h.ReplyToMail(id, req.Msg))
	})
	// One message in full — what a detail view and `mail show` ask for. A pure fetch: it never marks
	// anything (-> POST /mail/mark-read is the deliberate act that does).
	mux.HandleFunc("GET /mail", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			writeJSON(w, nil, fmt.Errorf("mail id must be a number, got %q", r.URL.Query().Get("id")))
			return
		}
		m, ok, err := h.MailBody(id)
		if err == nil && !ok {
			err = fmt.Errorf("no message %d", id)
		}
		writeJSON(w, m, err)
	})
	// The deliberate act — a dwell, an ENTER, `mail show` — that retires a message the user was sent.
	mux.HandleFunc("POST /mail/mark-read", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq // reuse: Name carries the mail id, Msg unused
		if !decode(w, r, &req) {
			return
		}
		id, err := strconv.ParseInt(req.Name, 10, 64)
		if err != nil {
			writeJSON(w, nil, fmt.Errorf("mail id must be a number, got %q", req.Name))
			return
		}
		writeJSON(w, okMsg{"marked read"}, h.MarkMailReadForUser(id))
	})
}
