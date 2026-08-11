// package: hub/store / tasks
// type:    adapter (SQLite, hub-owned)
// job:     the cached task read model (D15): the synced set (ReplaceTasks/UpsertTask),
// the claim queries the assigner reads (OpenLeaves, OpenContainers and its
// helpers), and the local approval/priority overlay layered on top.
// limits:  primitive columns only; mapping to/from issue.Task lives in the hub, and
// the schema (workflowSchema) stays put in workflow.go alongside its siblings.
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// Task is the cached read-model row; large fields land only on a detail read. It crosses the
// wire, so its definition lives in internal/api, under the name every caller here already uses.
type Task = api.Task

// ReplaceTasks swaps the cached set in one transaction; absent tasks are dropped.
func (p *ProjectStore) ReplaceTasks(tasks []Task) error {
	tx, err := p.s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM tasks WHERE project=?`, p.project); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range tasks {
		if _, err := tx.Exec(
			`INSERT INTO tasks (project,id,title,status,priority,type,labels,parent_id,description,url,updated_at,synced_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.project, t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels, t.ParentID, t.Description, t.URL, t.UpdatedAt, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertTask refreshes a single cached task in this project (point-of-use refresh).
func (p *ProjectStore) UpsertTask(t Task) error {
	_, err := p.s.db.Exec(`
		INSERT INTO tasks (project,id,title,status,priority,type,labels,parent_id,description,url,updated_at,synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project,id) DO UPDATE SET
			title=excluded.title, status=excluded.status, priority=excluded.priority,
			type=excluded.type, labels=excluded.labels, parent_id=excluded.parent_id,
			description=excluded.description, url=excluded.url, updated_at=excluded.updated_at,
			synced_at=excluded.synced_at`,
		p.project, t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels, t.ParentID, t.Description, t.URL,
		t.UpdatedAt, time.Now().UTC().Format(time.RFC3339))
	return err
}

// RemoveTask shows a close/scrap on the board without a full re-sync; the next sync
// rebuilds from the sources anyway, so this is only the intervening truth.
func (p *ProjectStore) RemoveTask(id string) error {
	if _, err := p.s.db.Exec(`DELETE FROM tasks WHERE project=? AND id=?`, p.project, id); err != nil {
		return err
	}
	return p.DeleteComments(id) // don't strand a scrapped task's comments
}

// taskCols is the shared projection: cached td fields plus the hub's approval overlay.
const taskCols = `t.id,t.title,t.status,t.priority,t.type,t.labels,t.parent_id,t.description,t.url,t.updated_at,
	COALESCE(a.status,''), COALESCE(a.comment,'')`

const taskFrom = ` FROM tasks t LEFT JOIN task_approval a ON a.task=t.id AND a.project=t.project`

// OpenTasks returns open, un-gated tasks, highest priority first.
func (p *ProjectStore) OpenTasks() ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=? AND t.status='open' AND (a.status IS NULL OR a.status='approved')
		ORDER BY CASE WHEN t.priority='' THEN 1 ELSE 0 END, t.priority, t.id`, p.project)
	if err != nil {
		return nil, fmt.Errorf("open tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// OpenLeaves returns the assigner's claimable tasks: open, approved, prioritised standalone
// leaves — no children of their own (those are packages -> OpenContainers) and no open parent —
// excluding any already held (agent_state.task, which is what keeps a gh-* issue from being
// handed out twice: it stays "open" on GitHub itself, with no source status to flip).
func (p *ProjectStore) OpenLeaves() ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=? AND t.status='open' AND (a.status IS NULL OR a.status='approved')
		  AND t.priority != ''
		  AND t.id NOT IN (SELECT parent_id FROM tasks WHERE project=? AND parent_id != '')
		  AND NOT EXISTS (SELECT 1 FROM tasks pp WHERE pp.project=t.project AND pp.id=t.parent_id AND pp.status='open')
		  AND t.id NOT IN (SELECT task FROM agent_state WHERE project=? AND task != '')
		ORDER BY t.priority, t.id`,
		p.project, p.project, p.project)
	if err != nil {
		return nil, fmt.Errorf("open leaves: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// OpenSubtasks returns the work left inside a feature: its open, approved descendants at ANY depth
// that have no open children of their own. Depth matters because a hierarchy is not always two
// levels — a direct child can itself be an epic, and handing one out as though it were a piece of
// work got it closed on the next checkpoint with its own children still open. Leaves only, for the
// reason OpenLeaves gives: the hierarchy is the unit of work, and its parents are not work.
func (p *ProjectStore) OpenSubtasks(parentID string) ([]Task, error) {
	rows, err := p.s.db.Query(`
		WITH RECURSIVE descendant(id) AS (
			SELECT id FROM tasks WHERE project=?1 AND parent_id=?2
			UNION
			SELECT t.id FROM tasks t JOIN descendant d ON t.parent_id=d.id WHERE t.project=?1
		)
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=?1 AND t.id IN (SELECT id FROM descendant)
		  AND t.status='open' AND (a.status IS NULL OR a.status='approved')
		  AND NOT EXISTS (SELECT 1 FROM tasks c WHERE c.project=t.project AND c.parent_id=t.id AND c.status='open')
		ORDER BY CASE WHEN t.priority='' THEN 1 ELSE 0 END, t.priority, t.id`, p.project, parentID)
	if err != nil {
		return nil, fmt.Errorf("open subtasks of %s: %w", parentID, err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// OpenChildIDs lists the DIRECT children of a task that are still open — what makes closing it a
// lie. Ids only: every caller either refuses on the count or names them back to a human.
func (p *ProjectStore) OpenChildIDs(parentID string) ([]string, error) {
	rows, err := p.s.db.Query(
		`SELECT id FROM tasks WHERE project=? AND parent_id=? AND status='open' ORDER BY id`,
		p.project, parentID)
	if err != nil {
		return nil, fmt.Errorf("open children of %s: %w", parentID, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetTask returns a single cached task in this project (with its approval overlay).
func (p *ProjectStore) GetTask(id string) (Task, bool, error) {
	row := p.s.db.QueryRow(`SELECT `+taskCols+taskFrom+` WHERE t.project=? AND t.id=?`, p.project, id)
	var t Task
	err := row.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &t.Type, &t.Labels, &t.ParentID, &t.Description, &t.URL, &t.UpdatedAt, &t.Approval, &t.ApprovalComment)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, fmt.Errorf("get task %s: %w", id, err)
	}
	return t, true, nil
}

// OpenContainers returns claimable packages: approved, prioritised, unheld tasks that HAD a child
// and have not landed. The SQL pre-filter only asks "any child at all" — whether it's workable,
// gated, or gone entirely is OpenSubtasks'/HasOpenDescendant's answer below, not a second opinion
// in SQL: a package with only a gated child stays excluded (the gate releases it later), while
// one whose every child has closed is offered anyway (nothing will EVER release it otherwise).
func (p *ProjectStore) OpenContainers() ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=? AND t.status NOT IN ('closed','approved','merged')
		  AND (a.status IS NULL OR a.status='approved')
		  AND t.priority != ''
		  AND EXISTS (SELECT 1 FROM tasks c WHERE c.project=t.project AND c.parent_id=t.id)
		  AND NOT EXISTS (SELECT 1 FROM tasks pp WHERE pp.project=t.project AND pp.id=t.parent_id AND pp.status='open')
		  AND t.id NOT IN (SELECT container FROM agent_state WHERE project=? AND container != '')
		ORDER BY t.priority, t.id`, p.project, p.project)
	if err != nil {
		return nil, fmt.Errorf("open containers: %w", err)
	}
	defer rows.Close()
	candidates, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(candidates))
	for _, c := range candidates {
		work, err := p.OpenSubtasks(c.ID)
		if err != nil {
			return nil, err
		}
		if len(work) > 0 {
			out = append(out, c) // real work to hand out
			continue
		}
		stuck, err := p.HasOpenDescendant(c.ID)
		if err != nil {
			return nil, err
		}
		if !stuck {
			out = append(out, c) // nothing left anywhere under it — claim it to finish and close it
		}
		// else: an open descendant exists somewhere but none of it is actionable right now (gated,
		// or itself a parent of gated work) — stays excluded until that gate clears.
	}
	return out, nil
}

// HasOpenDescendant reports an open descendant at ANY depth, gated or not — unlike OpenSubtasks
// ("workable now"), this tells a fully-closed tree apart from one that is merely gated.
func (p *ProjectStore) HasOpenDescendant(parentID string) (bool, error) {
	row := p.s.db.QueryRow(`
		WITH RECURSIVE descendant(id) AS (
			SELECT id FROM tasks WHERE project=?1 AND parent_id=?2
			UNION
			SELECT t.id FROM tasks t JOIN descendant d ON t.parent_id=d.id WHERE t.project=?1
		)
		SELECT EXISTS (
			SELECT 1 FROM tasks WHERE project=?1 AND id IN (SELECT id FROM descendant) AND status='open'
		)`, p.project, parentID)
	var has bool
	if err := row.Scan(&has); err != nil {
		return false, fmt.Errorf("has open descendant of %s: %w", parentID, err)
	}
	return has, nil
}

// AllTasks returns every cached task in this project with its approval overlay.
func (p *ProjectStore) AllTasks() ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=?
		ORDER BY CASE WHEN t.priority='' THEN 1 ELSE 0 END, t.priority, t.id`, p.project)
	if err != nil {
		return nil, fmt.Errorf("all tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTasks(rows *sql.Rows) ([]Task, error) {
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &t.Type, &t.Labels, &t.ParentID, &t.Description, &t.URL, &t.UpdatedAt, &t.Approval, &t.ApprovalComment); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetPriorityOverride records a priority we assign in our own db for this project.
func (p *ProjectStore) SetPriorityOverride(id, priority string) error {
	_, err := p.s.db.Exec(
		`INSERT INTO task_priority (project,id,priority) VALUES (?,?,?)
		 ON CONFLICT(project,id) DO UPDATE SET priority=excluded.priority`, p.project, id, priority)
	if err != nil {
		return fmt.Errorf("set priority override %s: %w", id, err)
	}
	return nil
}

// SetApproval records a task's approval state and comment in this project, now.
func (p *ProjectStore) SetApproval(task, status, comment string) error {
	_, err := p.s.db.Exec(
		`INSERT INTO task_approval (project,task,status,comment,at) VALUES (?,?,?,?,?)
		 ON CONFLICT(project,task) DO UPDATE SET status=excluded.status, comment=excluded.comment, at=excluded.at`,
		p.project, task, status, comment, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("set approval %s: %w", task, err)
	}
	return nil
}

// ClearApproval drops a task's approval gate. A gate is a question awaiting the user, so it is
// removed rather than answered when the task it asks about ends.
func (p *ProjectStore) ClearApproval(task string) error {
	_, err := p.s.db.Exec(`DELETE FROM task_approval WHERE project=? AND task=?`, p.project, task)
	if err != nil {
		return fmt.Errorf("clear approval %s: %w", task, err)
	}
	return nil
}

// GetApproval returns a task's approval status and comment in this project.
func (p *ProjectStore) GetApproval(task string) (status, comment string) {
	_ = p.s.db.QueryRow(`SELECT status, comment FROM task_approval WHERE project=? AND task=?`, p.project, task).Scan(&status, &comment)
	return status, comment
}

// PriorityOverrides returns id→priority for this project's locally-assigned priorities.
func (p *ProjectStore) PriorityOverrides() (map[string]string, error) {
	rows, err := p.s.db.Query(`SELECT id, priority FROM task_priority WHERE project=?`, p.project)
	if err != nil {
		return nil, fmt.Errorf("priority overrides: %w", err)
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var id, pr string
		if err := rows.Scan(&id, &pr); err != nil {
			return nil, err
		}
		m[id] = pr
	}
	return m, rows.Err()
}
