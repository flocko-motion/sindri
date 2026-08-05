// package: hub/workflow / importtd
// job:     carry a repo's existing td backlog into owned_tasks, once, the first time
// the hub syncs that project — so taking td out costs nobody their tasks.
// type:    logic (one-way migration)
// limits:  reads td's SQLite and writes owned rows; the td CLI is never invoked.
package workflow

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/tasks/td"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// tdImportKey marks a project's import as done. Once only: a second pass would resurrect every task
// closed or scrapped since, because td still holds the state it had at migration.
func tdImportKey(project string) string { return "imported:td:" + project }

// importTdOnce moves a repo's td tasks into owned_tasks the first time this project is synced.
// A repo with no td store, or one already imported, returns immediately.
func (e *Engine) importTdOnce(project, root string) error {
	if _, seen, err := e.store.GetMeta(tdImportKey(project)); err != nil || seen {
		return err
	}
	if root == "" || !td.HasStore(root) {
		return e.store.SetMeta(tdImportKey(project), "no td store")
	}
	existing, err := td.Tasks(root, task.FilterAll)
	if err != nil {
		return fmt.Errorf("read td tasks for import: %w", err)
	}
	ps := e.store.For(project)
	for _, t := range existing {
		if !strings.HasPrefix(t.ID, OwnedPrefix) {
			continue
		}
		if err := ps.PutOwnedTask(store.OwnedTask{
			ID: t.ID, Title: t.Title, Status: t.Status, Priority: t.Priority, Type: t.Type,
			Labels: strings.Join(t.Labels, ","), Description: t.Description,
		}); err != nil {
			return fmt.Errorf("import %s: %w", t.ID, err)
		}
		if err := ps.SetParent(t.ID, t.ParentID); err != nil {
			return fmt.Errorf("import parent of %s: %w", t.ID, err)
		}
	}
	fmt.Fprintf(os.Stderr, "hub: imported %d td task(s) into sindri's own store for %s\n", len(existing), project)
	return e.store.SetMeta(tdImportKey(project), fmt.Sprintf("%d task(s)", len(existing)))
}
