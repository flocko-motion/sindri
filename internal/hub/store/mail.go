// package: hub/store / mail
// type:    adapter (persistence for the agent mailbox)
// job:     the durable record of every message an agent MUST read — append-only, with a
// read flag and no delete path, so reading marks a message rather than consuming
// it and the history survives. Global reads (the fleet-wide Mail view) sit
// alongside the project-keyed ones, as PRs do.
// limits:  rows only. Who sends what, and which messages are mail rather than a push,
// is the hub's (-> sd-7b317b's sender classification).
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// Mail is one message in an agent's mailbox; it crosses the wire, so it is internal/api.Mail
// under the name every existing caller here already uses.
type Mail = api.Mail

// mailSchema is the mailbox: APPEND-ONLY, no delete or expiry — reading only sets read_at.
const mailSchema = `
CREATE TABLE IF NOT EXISTS mail (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  project TEXT NOT NULL,
  agent   TEXT NOT NULL,           -- the recipient
  sender  TEXT NOT NULL DEFAULT '',-- hub | user | reviewer | an agent's name
  body    TEXT NOT NULL DEFAULT '',
  sent_at TEXT NOT NULL DEFAULT '',
  read_at TEXT NOT NULL DEFAULT '',-- '' = unread; set when the agent reads it, never cleared
  pushed  INTEGER NOT NULL DEFAULT 0, -- the same message was also injected, so it may have been seen live
  -- The message this one answers (0 = starts a thread), so an exchange reads as an exchange rather
  -- than as scattered rows the recipient has to match up by hand.
  in_reply_to INTEGER NOT NULL DEFAULT 0,
  -- The hub has since told the recipient this message is waiting. NOT the pushed column, which says the
  -- text itself was injected at delivery: one means "it may have acted on this already", the other "it
  -- has been told there is something to read". Per-message and durable, so a restart announces nothing
  -- a second time.
  notified INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS mail_agent ON mail (project, agent, id);
`

const mailCols = `SELECT id, project, agent, sender, body, sent_at, read_at, pushed, in_reply_to FROM mail`

// AddMail records a message an agent must read, returning the stored row. pushed says whether it was
// also injected — "pushed and possibly missed" and "sitting here unread" are different diagnoses.
func (p *ProjectStore) AddMail(agent, sender, body string, pushed bool, inReplyTo int64) (Mail, error) {
	m := Mail{Project: p.project, Agent: agent, Sender: sender, Body: body,
		SentAt: time.Now().UTC().Format(time.RFC3339), Pushed: pushed, InReplyTo: inReplyTo}
	res, err := p.s.db.Exec(
		`INSERT INTO mail (project, agent, sender, body, sent_at, pushed, in_reply_to) VALUES (?,?,?,?,?,?,?)`,
		m.Project, m.Agent, m.Sender, m.Body, m.SentAt, m.Pushed, m.InReplyTo)
	if err != nil {
		return Mail{}, fmt.Errorf("add mail for %s/%s: %w", p.project, agent, err)
	}
	if m.ID, err = res.LastInsertId(); err != nil {
		return Mail{}, fmt.Errorf("add mail for %s/%s: %w", p.project, agent, err)
	}
	return m, nil
}

// MarkMailRead stamps a message as read, now — the row stays (-> mailSchema). Re-reading keeps the
// first stamp, which is the one that answers "when did it learn".
func (p *ProjectStore) MarkMailRead(id int64) error {
	res, err := p.s.db.Exec(`UPDATE mail SET read_at=? WHERE id=? AND project=? AND read_at=''`,
		time.Now().UTC().Format(time.RFC3339), id, p.project)
	if err != nil {
		return fmt.Errorf("mark mail %d read: %w", id, err)
	}
	// Only the FIRST read is an event: re-reading changes nothing, and a trail that grew every time
	// somebody looked would bury the delivery it exists to explain.
	if n, aerr := res.RowsAffected(); aerr == nil && n > 0 {
		_ = p.LogMail(id, MailRead, "")
	}
	return nil
}

// MarkMailPushed is set only when the injection actually SUCCEEDS, never from intent alone.
func (p *ProjectStore) MarkMailPushed(id int64) error {
	_, err := p.s.db.Exec(`UPDATE mail SET pushed=1 WHERE id=? AND project=?`, id, p.project)
	if err != nil {
		return fmt.Errorf("mark mail %d pushed: %w", id, err)
	}
	return nil
}

// UnreadMail is an agent's unread messages, OLDEST FIRST — a later one often supersedes an earlier one.
func (p *ProjectStore) UnreadMail(agent string) ([]Mail, error) {
	return queryMail(p.s.db, mailCols+` WHERE project=? AND agent=? AND read_at='' ORDER BY id`,
		p.project, agent)
}

// Mail returns every message in this project, newest first — mail list's whole-project scope.
func (p *ProjectStore) Mail() ([]Mail, error) {
	return queryMail(p.s.db, mailCols+` WHERE project=? ORDER BY id DESC`, p.project)
}

// UnannouncedMail counts what a nudge is about — per message, not a timer, so nothing is announced twice.
func (p *ProjectStore) UnannouncedMail(agent string) (unannounced, unread int, err error) {
	err = p.s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(notified = 0 AND pushed = 0), 0) FROM mail WHERE project=? AND agent=? AND read_at=''`,
		p.project, agent).Scan(&unread, &unannounced)
	if err != nil {
		return 0, 0, fmt.Errorf("unannounced mail for %s: %w", agent, err)
	}
	return unannounced, unread, nil
}

// MarkMailAnnounced records that the agent has been told about everything now waiting — one statement, so
// marking cannot outrun what was announced and lose a message for ever.
func (p *ProjectStore) MarkMailAnnounced(agent string) error {
	// The ids first: the UPDATE is what makes them stop matching, so reading after it returns none
	// and the trail would record an announcement against nothing.
	ids, _ := p.unannouncedIDs(agent)
	_, err := p.s.db.Exec(
		`UPDATE mail SET notified=1 WHERE project=? AND agent=? AND read_at='' AND notified=0`,
		p.project, agent)
	if err != nil {
		return fmt.Errorf("mark mail announced for %s: %w", agent, err)
	}
	for _, id := range ids {
		_ = p.LogMail(id, MailAnnounced, "")
	}
	return nil
}

// unannouncedIDs are the messages MarkMailAnnounced is about to claim, read before it claims them.
func (p *ProjectStore) unannouncedIDs(agent string) ([]int64, error) {
	rows, err := p.s.db.Query(
		`SELECT id FROM mail WHERE project=? AND agent=? AND read_at='' AND notified=0`, p.project, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// UnreadMailCount is how many messages an agent has not read — what the directive reminds it of.
func (p *ProjectStore) UnreadMailCount(agent string) (int, error) {
	var n int
	err := p.s.db.QueryRow(`SELECT COUNT(*) FROM mail WHERE project=? AND agent=? AND read_at=''`,
		p.project, agent).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("unread mail for %s: %w", agent, err)
	}
	return n, nil
}

// UnreadMailByAgent is unread mail per project and agent, in one query for the whole fleet — the
// board reads it per row, and a query per agent would be paid per render (-> ActiveReviewers).
func (s *Store) UnreadMailByAgent() (map[string]map[string]int, error) {
	rows, err := s.db.Query(
		`SELECT project, agent, COUNT(*) FROM mail WHERE read_at='' GROUP BY project, agent`)
	if err != nil {
		return nil, fmt.Errorf("unread mail by agent: %w", err)
	}
	defer rows.Close()
	out := map[string]map[string]int{}
	for rows.Next() {
		var project, agent string
		var n int
		if err := rows.Scan(&project, &agent, &n); err != nil {
			return nil, fmt.Errorf("scan unread tally: %w", err)
		}
		if out[project] == nil {
			out[project] = map[string]int{}
		}
		out[project][agent] = n
	}
	return out, rows.Err()
}

// AllMail returns the newest limit messages across every project, newest first — the fleet view's
// window, which therefore keeps the recent end. A non-positive limit returns everything.
func (s *Store) AllMail(limit int) ([]Mail, error) {
	q := mailCols + ` ORDER BY id DESC`
	var args []any
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	return queryMail(s.db, q, args...)
}

// MailTallies is every board number in ONE PASS over the whole table — rebuilt per notify per client.
func (s *Store) MailTallies() (total, unread, userUnread int, byProject map[string]int, err error) {
	byProject = map[string]int{}
	rows, qerr := s.db.Query(
		`SELECT project, COUNT(*), SUM(read_at = ''), SUM(read_at = '' AND agent = ?) FROM mail GROUP BY project`,
		api.SenderUser)
	if qerr != nil {
		return 0, 0, 0, nil, fmt.Errorf("mail tallies: %w", qerr)
	}
	defer rows.Close()
	for rows.Next() {
		var project string
		var n, u, mine int
		if err := rows.Scan(&project, &n, &u, &mine); err != nil {
			return 0, 0, 0, nil, fmt.Errorf("scan mail tally: %w", err)
		}
		total, unread, userUnread, byProject[project] = total+n, unread+u, userUnread+mine, u
	}
	return total, unread, userUnread, byProject, rows.Err()
}

// NotesToUserSince counts UNPROMPTED notes only — a reply never counts against the ceiling.
func (s *Store) NotesToUserSince(t time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM mail WHERE agent=? AND in_reply_to=0 AND sent_at >= ?`,
		api.SenderUser, t.UTC().Format(time.RFC3339)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("notes to the user since %s: %w", t, err)
	}
	return n, nil
}

// MailByID returns one message with its FULL body, from any project — what the detail view and
// `mail show` read, since the fleet list carries only a preview of each body.
func (s *Store) MailByID(id int64) (Mail, bool, error) {
	m, err := scanMail(s.db.QueryRow(mailCols+` WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return Mail{}, false, nil
	}
	if err != nil {
		return Mail{}, false, fmt.Errorf("mail %d: %w", id, err)
	}
	return m, true, nil
}

func queryMail(db *sql.DB, q string, args ...any) ([]Mail, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("mail: %w", err)
	}
	defer rows.Close()
	var out []Mail
	for rows.Next() {
		m, err := scanMail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanMail(row scanner) (Mail, error) {
	var m Mail
	err := row.Scan(&m.ID, &m.Project, &m.Agent, &m.Sender, &m.Body, &m.SentAt, &m.ReadAt, &m.Pushed, &m.InReplyTo)
	return m, err
}
