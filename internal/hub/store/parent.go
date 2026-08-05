// package: hub/store / parent
// type:    persistence (SQLite, hub-owned)
// job:     record which task is whose parent, for every task whatever owns its text —
// the hierarchy is sindri's own, so an openspec change or a GitHub issue takes
// part in it like any other.
// limits:  storage and lookup; validating a parent (it exists, no cycle) is the
// workflow's, and the sync applies these links to the read model.
package store

import "fmt"

// SetParent records a task's parent, or clears it when parentID is empty.
func (p *ProjectStore) SetParent(id, parentID string) error {
	if parentID == "" {
		return p.ClearParent(id)
	}
	_, err := p.s.db.Exec(
		`INSERT INTO task_parent (project,id,parent_id) VALUES (?,?,?)
		 ON CONFLICT(project,id) DO UPDATE SET parent_id=excluded.parent_id`, p.project, id, parentID)
	if err != nil {
		return fmt.Errorf("set parent of %s: %w", id, err)
	}
	return nil
}

// ClearParent detaches a task, making it a root again.
func (p *ProjectStore) ClearParent(id string) error {
	if _, err := p.s.db.Exec(`DELETE FROM task_parent WHERE project=? AND id=?`, p.project, id); err != nil {
		return fmt.Errorf("clear parent of %s: %w", id, err)
	}
	return nil
}

// ParentLinks is every child→parent link in the project, for the sync to lay over the read model.
func (p *ProjectStore) ParentLinks() (map[string]string, error) {
	rows, err := p.s.db.Query(`SELECT id, parent_id FROM task_parent WHERE project=?`, p.project)
	if err != nil {
		return nil, fmt.Errorf("parent links: %w", err)
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var id, parent string
		if err := rows.Scan(&id, &parent); err != nil {
			return nil, err
		}
		m[id] = parent
	}
	return m, rows.Err()
}

// ParentOf is one task's parent, empty when it is a root.
func (p *ProjectStore) ParentOf(id string) string {
	var parent string
	if err := p.s.db.QueryRow(
		`SELECT parent_id FROM task_parent WHERE project=? AND id=?`, p.project, id).Scan(&parent); err != nil {
		return ""
	}
	return parent
}
