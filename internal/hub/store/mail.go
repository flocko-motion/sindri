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

// mailSchema is the mailbox. APPEND-ONLY BY DESIGN — no delete, no expiry: reading sets read_at, so
// the rows are the record of what an agent was told. Affordable because push-only traffic (a nudge, a
// broadcast) is never stored here at all, leaving one-shot consequence to accumulate.
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
  in_reply_to INTEGER NOT NULL DEFAULT 0
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
	_, err := p.s.db.Exec(`UPDATE mail SET read_at=? WHERE id=? AND project=? AND read_at=''`,
		time.Now().UTC().Format(time.RFC3339), id, p.project)
	if err != nil {
		return fmt.Errorf("mark mail %d read: %w", id, err)
	}
	return nil
}

// MarkMailPushed records that the wake for this message actually landed. Set after the injection
// SUCCEEDS, never from the sender's intent: a row claiming a push that never reached a down agent
// would erase the difference between "it may have acted on this already" and "nothing reached it".
func (p *ProjectStore) MarkMailPushed(id int64) error {
	_, err := p.s.db.Exec(`UPDATE mail SET pushed=1 WHERE id=? AND project=?`, id, p.project)
	if err != nil {
		return fmt.Errorf("mark mail %d pushed: %w", id, err)
	}
	return nil
}

// UnreadMail is an agent's unread messages, OLDEST FIRST — the order they were sent is the order
// they make sense in, since a later message often supersedes an earlier one.
func (p *ProjectStore) UnreadMail(agent string) ([]Mail, error) {
	return queryMail(p.s.db, mailCols+` WHERE project=? AND agent=? AND read_at='' ORDER BY id`,
		p.project, agent)
}

// NewestUnreadMail is an agent's unread count and the id of its newest unread message. The id is what a
// nudge is keyed on: telling an agent once about what is waiting is the point, and NEW mail changes the
// id, which is what makes a second nudge honest rather than a repeat.
func (p *ProjectStore) NewestUnreadMail(agent string) (newest int64, count int, err error) {
	err = p.s.db.QueryRow(
		`SELECT COALESCE(MAX(id), 0), COUNT(*) FROM mail WHERE project=? AND agent=? AND read_at=''`,
		p.project, agent).Scan(&newest, &count)
	if err != nil {
		return 0, 0, fmt.Errorf("newest unread mail for %s: %w", agent, err)
	}
	return newest, count, nil
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

// MailTallies is every number the board needs about the mailbox: size, unread, the user's own unread,
// and unread per project — counted here, not over the window, since a badge from a window stops rising.
//
// ONE PASS: the board is rebuilt per notify per client over a table that grows for the life of the
// machine, so a second scan for the user's share would be paid on every read.
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

// NotesToUserSince counts what the whole fleet has sent the user since t — the fleet-wide ceiling's
// only input. Counted over the mailbox rather than a running tally, so there is nothing to drift: the
// rows are the record, and a rolling window has no cliff for a queue to build against.
func (s *Store) NotesToUserSince(t time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM mail WHERE agent=? AND sent_at >= ?`,
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
