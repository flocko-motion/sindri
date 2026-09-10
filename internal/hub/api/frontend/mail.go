// package: hub/api/frontend / mail
// type:    adapter (HTTP transport)
// job:     the routes that carry a message to an agent or read one back — PUSH it into the
// live session, or fetch a message from its mailbox in full. Grouped because they are
// the two halves of one question (must it be read, must it wake) rather than two
// unrelated endpoints.
// limits:  pure transport over the hub's handles; the classification is the sender's
// (-> mail.Delivery) and the mailbox is the store's.
package frontend

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/api/serve"
	"net/http"
	"strconv"
)

// messageRoutes registers the message endpoints on mux.
func messageRoutes(mux *http.ServeMux, h Hub) {
	mux.HandleFunc("POST /tell", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"delivered"}, h.Agents().Tell(serve.Detached(r), h.AgentReq(r, req.Name), req.Name, req.Msg, req.Source, req.SignedOut))
	})
	// The other half of the pair: mail waits to be read and does not interrupt, where /tell wakes the
	// agent now and is lost if it is not there. Two routes, because the choice is the user's.
	mux.HandleFunc("POST /agent/mail", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq
		if !serve.Decode(w, r, &req) {
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"mailed"}, h.Mail().MailAgent(h.AgentReq(r, req.Name), req.Name, req.Msg))
	})
	// The user answering an agent's message, from either front-end: the recipient comes from the row.
	mux.HandleFunc("POST /mail/reply", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq // Name carries the message id, Msg the reply
		if !serve.Decode(w, r, &req) {
			return
		}
		id, err := strconv.ParseInt(req.Name, 10, 64)
		if err != nil {
			serve.WriteJSON(w, nil, fmt.Errorf("mail id must be a number, got %q", req.Name))
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"replied"}, h.Mail().ReplyToMail(id, req.Msg))
	})
	// One message in full — what a detail view and `mail show` ask for. A pure fetch: it never marks
	// anything (-> POST /mail/mark-read is the deliberate act that does).
	mux.HandleFunc("GET /mail", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			serve.WriteJSON(w, nil, fmt.Errorf("mail id must be a number, got %q", r.URL.Query().Get("id")))
			return
		}
		m, ok, err := h.Mail().MailBody(id)
		if err == nil && !ok {
			err = fmt.Errorf("no message %d", id)
		}
		serve.WriteJSON(w, m, err)
	})
	// The deliberate act — a dwell, an ENTER, `mail show` — that retires a message the user was sent.
	mux.HandleFunc("POST /mail/mark-read", func(w http.ResponseWriter, r *http.Request) {
		var req TellReq // reuse: Name carries the mail id, Msg unused
		if !serve.Decode(w, r, &req) {
			return
		}
		id, err := strconv.ParseInt(req.Name, 10, 64)
		if err != nil {
			serve.WriteJSON(w, nil, fmt.Errorf("mail id must be a number, got %q", req.Name))
			return
		}
		serve.WriteJSON(w, serve.OKMsg{"marked read"}, h.Mail().MarkMailReadForUser(id))
	})
	// The same act over the whole backlog, for a Mail tab read down to nothing but the marker.
	mux.HandleFunc("POST /mail/mark-all-read", func(w http.ResponseWriter, r *http.Request) {
		n, err := h.Mail().MarkAllUserMailRead()
		serve.WriteJSON(w, serve.OKMsg{fmt.Sprintf("marked %d read", n)}, err)
	})
}
