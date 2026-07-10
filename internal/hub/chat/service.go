// package: hub/chat / service
// type:    logic (the chatroom relay — star topology)
// job:     the user's one chatroom: membership, the message relay (record + forward
//          to every participant), the in-chat slash commands, and the required-
//          participant presence lock. Agent delivery + board notify go through a
//          Delivery port, so this package doesn't depend on the hub.
// limits:  persistence is store/chat.go; HTTP/CLI/TUI wiring lives in the hub and
//          cmd/*; no tmux/container/http here.
package chat

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// Delivery is the hub-provided port for reaching agents and waking live views —
// the only coupling back to the hub, so chat logic stays independent of tmux and
// the events bus.
type Delivery interface {
	Inject(project, name, text string) error // type a line into an agent's session
	Running(project, name string) bool        // is the agent's pod up?
	Notify()                                   // wake the board / live views
}

const (
	user   = "user"   // sender label for the human's messages
	system = "system" // sender label for the hub's own lines (join/leave, command replies)

	transcriptLimit = 200              // how much history a snapshot / live view carries
	presenceTTL     = 20 * time.Second // room stays unlocked this long after the last heartbeat
	maxLen          = 4000             // per-message cap (deep talk, but not a novel / huge inject)

	helpText = "commands: /add <agent> (alias /invite) · /remove <agent> (alias /kick) · /who (list members) · /help. Anything not starting with / is sent to everyone in the room."
)

// Message notices pushed on membership changes / relaunch. Exported so the hub can
// re-announce membership when an agent relaunches (MsgReminder).
const (
	MsgWelcome  = "[hub] You've been added to the chatroom. Use `sindri chat <message>` to emit a message to everybody in the room; you'll also receive the others' messages here, prefixed [chat]. Use it to coordinate issues with the other agents — tell them what you're working on and listen to what they're working on. The user will lead the discussion to answer an open question as a team."
	MsgReminder = "[hub] You're in the chatroom: `sindri chat <message>` talks to everyone in the room, and their messages arrive here prefixed [chat]."
	MsgRemoved  = "[hub] You've been removed from the chatroom — `sindri chat` is no longer available. Carry on with your work."
)

// errTooLong is returned when a message exceeds maxLen — actionable feedback for
// whoever sent it, so callers surface it to the sender.
var errTooLong = fmt.Errorf("message too long — keep it under %d characters (split a longer one into parts)", maxLen)

// Service is the chatroom relay. Construct with New; the hub owns the instance.
type Service struct {
	store *store.Store
	d     Delivery

	mu   sync.Mutex
	seen time.Time // last user heartbeat — presence for the required-participant lock
}

// New builds the chat service over the store and a delivery port.
func New(st *store.Store, d Delivery) *Service { return &Service{store: st, d: d} }

// Add puts an agent in the chatroom and greets it. Errors if the agent doesn't
// exist or is already a member. The greeting is best-effort (a stopped agent gets
// reminded on relaunch).
func (s *Service) Add(project, name string) error {
	if _, ok, err := s.store.For(project).GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	member, err := s.store.ChatIsMember(project, name)
	if err != nil {
		return err
	}
	if member {
		return fmt.Errorf("%s is already in the chatroom", name)
	}
	if err := s.store.ChatAdd(project, name); err != nil {
		return err
	}
	s.deliver(project, name, MsgWelcome)
	_, err = s.broadcast("", system, name+" joined the chatroom") // announce (IRC-style)
	return err
}

// Remove takes an agent out of the chatroom and tells it so. Errors if it wasn't a
// member.
func (s *Service) Remove(project, name string) error {
	was, err := s.store.ChatRemove(project, name)
	if err != nil {
		return err
	}
	if !was {
		return fmt.Errorf("%s is not in the chatroom", name)
	}
	s.deliver(project, name, MsgRemoved)
	_, err = s.broadcast("", system, name+" left the chatroom")
	return err
}

// IsMember reports whether an agent is currently in the chatroom.
func (s *Service) IsMember(project, name string) (bool, error) {
	return s.store.ChatIsMember(project, name)
}

// Members returns the current room roster.
func (s *Service) Members() ([]store.ChatMember, error) { return s.store.ChatMembers() }

// Transcript returns the recent room transcript (oldest first); a non-positive
// limit uses the default window.
func (s *Service) Transcript(limit int) ([]store.ChatMessage, error) {
	if limit <= 0 {
		limit = transcriptLimit
	}
	return s.store.ChatTranscript(limit)
}

// Heartbeat records that the user is present (called periodically by `chat join`
// and the TUI chat tab). Presence keeps the room unlocked.
func (s *Service) Heartbeat() {
	s.mu.Lock()
	s.seen = time.Now()
	s.mu.Unlock()
}

// Present reports whether the user is in the room (a heartbeat within presenceTTL).
func (s *Service) Present() bool {
	s.mu.Lock()
	seen := s.seen
	s.mu.Unlock()
	return !seen.IsZero() && time.Since(seen) < presenceTTL
}

// Say forwards a message from the user (the discussion leader) to the room.
func (s *Service) Say(msg string) (store.ChatMessage, error) { return s.broadcast("", user, msg) }

// UserMessage handles one line the user submits: a "/command" the hub executes,
// else a broadcast. Only this (the user's) path interprets commands, so agents
// (via Cmd) can never change membership.
func (s *Service) UserMessage(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return fmt.Errorf("empty message")
	}
	s.Heartbeat() // the user just posted — they're present
	if strings.HasPrefix(line, "/") {
		return s.command(line)
	}
	_, err := s.Say(line)
	return err
}

// Cmd is the agent-facing `chat` verb: forward the agent's message to the room.
// The registry hides it from non-members; here we enforce the presence lock.
func (s *Service) Cmd(c registry.Caller, args []string, out io.Writer) (int, error) {
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		fmt.Fprintln(out, "usage: chat <message...>")
		return 2, nil
	}
	if !s.Present() {
		fmt.Fprintln(out, "the chatroom is locked — the user has stepped away, so your message wasn't sent. Wait, then send it again once they're back.")
		return 1, nil
	}
	if _, err := s.broadcast(c.Project, c.Agent, msg); err != nil {
		if errors.Is(err, errTooLong) { // agent's to act on — show it (not neutralized upstream)
			fmt.Fprintln(out, err.Error())
			return 1, nil
		}
		return 1, err
	}
	fmt.Fprintln(out, "sent to the chatroom")
	return 0, nil
}

// command parses and runs a "/command"; its result is a system line in the
// transcript (visible to the user, not injected into agents).
func (s *Service) command(line string) error {
	fields := strings.Fields(line)
	verb := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	args := fields[1:]
	switch verb {
	case "add", "invite":
		return s.membershipCmd(args, true)
	case "remove", "rm", "kick":
		return s.membershipCmd(args, false)
	case "who", "names", "members":
		return s.systemReply(s.whoLine())
	case "help", "?":
		return s.systemReply(helpText)
	case "quit", "q", "leave", "part", "exit":
		return s.systemReply("to leave: press Ctrl-D or type /quit in `sindri chat join`; in the TUI, just switch tabs.")
	default:
		return s.systemReply("unknown command /" + verb + " — type /help for the list")
	}
}

// membershipCmd runs /add or /remove for one or more agents, resolving each by its
// globally-unique name. Add/Remove announce the join/leave; only failures reply.
func (s *Service) membershipCmd(args []string, add bool) error {
	if len(args) == 0 {
		if add {
			return s.systemReply("usage: /add <agent>… (alias /invite)")
		}
		return s.systemReply("usage: /remove <agent>… (alias /kick)")
	}
	for _, name := range args {
		project, ok, err := s.findAgentProject(name)
		if err != nil {
			return err
		}
		if !ok {
			if rerr := s.systemReply(fmt.Sprintf("no agent named %q", name)); rerr != nil {
				return rerr
			}
			continue
		}
		var aerr error
		if add {
			aerr = s.Add(project, name)
		} else {
			aerr = s.Remove(project, name)
		}
		if aerr != nil {
			if rerr := s.systemReply(aerr.Error()); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}

// whoLine renders the /who reply: the roster, or a nudge when empty.
func (s *Service) whoLine() string {
	members, err := s.store.ChatMembers()
	if err != nil {
		return "couldn't read the member list"
	}
	if len(members) == 0 {
		return "the chatroom is empty — /add <agent> to bring someone in"
	}
	parts := make([]string, len(members))
	for i, m := range members {
		if m.Role != "" {
			parts[i] = fmt.Sprintf("%s (%s)", m.Name, m.Role)
		} else {
			parts[i] = m.Name
		}
	}
	return fmt.Sprintf("members (%d): %s", len(members), strings.Join(parts, ", "))
}

// systemReply records a hub reply in the transcript (not injected into agents) and
// wakes the user's live views.
func (s *Service) systemReply(text string) error {
	if _, err := s.store.ChatAppend(system, text); err != nil {
		return err
	}
	s.d.Notify()
	return nil
}

// findAgentProject resolves an agent's project by its (globally unique) name.
func (s *Service) findAgentProject(name string) (string, bool, error) {
	agents, err := s.store.AllAgents()
	if err != nil {
		return "", false, err
	}
	for _, a := range agents {
		if a.Name == name {
			return a.Project, true, nil
		}
	}
	return "", false, nil
}

// broadcast is the star centre: record the message, forward it to every member
// agent (skipping the sender), and wake the user's live views.
func (s *Service) broadcast(senderProject, senderName, body string) (store.ChatMessage, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return store.ChatMessage{}, fmt.Errorf("empty message")
	}
	if utf8.RuneCountInString(body) > maxLen {
		return store.ChatMessage{}, errTooLong
	}
	msg, err := s.store.ChatAppend(senderName, body)
	if err != nil {
		return store.ChatMessage{}, err
	}
	members, err := s.store.ChatMembers()
	if err != nil {
		return store.ChatMessage{}, err
	}
	line := fmt.Sprintf("[chat] %s: %s", senderName, body)
	for _, m := range members {
		if m.Project == senderProject && m.Name == senderName {
			continue // the sender already has its own words
		}
		s.deliver(m.Project, m.Name, line)
	}
	s.d.Notify()
	return msg, nil
}

// deliver injects one chat line into an agent's session and logs it. Best-effort:
// an offline agent is skipped, an injection error is logged upstream via the port's
// own reporting — a chat line never fails a broadcast (it stays in the transcript).
func (s *Service) deliver(project, name, line string) {
	if !s.d.Running(project, name) {
		return
	}
	if err := s.d.Inject(project, name, line); err != nil {
		fmt.Fprintf(os.Stderr, "hub: chat delivery to %s/%s failed: %v\n", project, name, err)
		return
	}
	if err := s.store.For(project).Log(name, "chat", line); err != nil {
		fmt.Fprintf(os.Stderr, "hub: logging chat for %s/%s failed: %v\n", project, name, err)
	}
}
