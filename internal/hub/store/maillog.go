// package: hub/store / mail log
// type:    adapter (SQLite, hub-owned)
// job:     the lifecycle of one message — every push attempted, landed or refused, every
// announcement, and the read — as an append-only trail beside the row's current flags.
// limits:  rows only; what each event MEANS, and when to write one, are the hub's.
package store

import (
	"fmt"
	"time"
)

// mailLogSchema mirrors pr_events, which already answers "how did this get here" for a PR. The mail
// row holds four flags and no history, so three pushes that never landed left `pushed=0` — the same
// value a message nobody ever pushed carries — and the reason nowhere at all.
const mailLogSchema = `
CREATE TABLE IF NOT EXISTS mail_events (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  project TEXT NOT NULL,
  mail    INTEGER NOT NULL,
  ts      TEXT NOT NULL,
  type    TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS mail_events_mail ON mail_events (project, mail, id);
`

// The lifecycle vocabulary. Each names something that HAPPENED, so a reader reconstructs the
// sequence rather than inferring it from flags that only ever report the latest state.
const (
	MailPushLanded = "push-landed" // send-keys accepted the text
	MailPushFailed = "push-failed" // nothing was typed; the payload says why
	MailAnnounced  = "announced"   // the recipient was told a message is waiting
	MailRead       = "read"        // the recipient read it
)

// LogMail appends one lifecycle event. Best-effort by design: losing the trail must never cost the
// delivery it describes.
func (p *ProjectStore) LogMail(mailID int64, typ, payload string) error {
	if mailID == 0 {
		return nil // push-only traffic keeps no row, so there is nothing to hang a history on
	}
	_, err := p.s.db.Exec(
		`INSERT INTO mail_events (project, mail, ts, type, payload) VALUES (?,?,?,?,?)`,
		p.project, mailID, time.Now().UTC().Format(time.RFC3339), typ, payload)
	if err != nil {
		return fmt.Errorf("log mail event %s for %d: %w", typ, mailID, err)
	}
	return nil
}

// MailEvents is one message's lifecycle, oldest first — the order it happened in.
func (p *ProjectStore) MailEvents(mailID int64) ([]Event, error) {
	rows, err := p.s.db.Query(
		`SELECT id, mail, ts, type, payload FROM mail_events WHERE project=? AND mail=? ORDER BY id`,
		p.project, mailID)
	if err != nil {
		return nil, fmt.Errorf("mail events for %d: %w", mailID, err)
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var mail int64
		if err := rows.Scan(&e.ID, &mail, &e.TS, &e.Type, &e.Payload); err != nil {
			return nil, fmt.Errorf("mail events for %d: %w", mailID, err)
		}
		e.Agent = fmt.Sprintf("ml-%d", mail)
		out = append(out, e)
	}
	return out, rows.Err()
}
