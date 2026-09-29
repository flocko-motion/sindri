// package: hub/flowtest / source
// type:    assembly (a task source that owns ids the hub does not)
// job:     stand in for openspec or GitHub — a source whose ids live in the REPO, so a test can
// show what the hub records when a status it cannot see has moved.
// limits:  the seam. Which ids belong to whom is each real source's own answer.
package flowtest

import (
	"strings"

	"github.com/flo-at/sindri/internal/hub/world/task"
)

// ForeignSource claims every os- id and finishes it silently, the way a repo-held status does.
type ForeignSource struct{}

// The rest of the port, answered as blandly as it can be: this stub exists for Finish alone, and a
// test that needed any of these would be testing the fake.
func (ForeignSource) Name() string { return "foreign" }

// Enabled: always, so nothing has to configure the fake.
func (ForeignSource) Enabled(string) bool { return true }

// ToolMissing: never — the fake shells out to nothing.
func (ForeignSource) ToolMissing(string) bool { return false }

// Tasks contributes none: what this source owns is seeded straight into the store.
func (ForeignSource) Tasks(string, bool) ([]task.Task, error) { return nil, nil }

// OnMerged does nothing, which is the point — a repo-held status the hub cannot see move.
func (ForeignSource) OnMerged(string, string, string) error { return nil }

// Finish claims the id if it is one of this source's, which is what the dispatch reads.
func (ForeignSource) Finish(_, id string, _ bool) (bool, error) {
	return strings.HasPrefix(id, "os-"), nil
}

// Comments reports the source keeps no thread of its own.
func (ForeignSource) Comments(string, string) ([]task.Comment, bool, error) { return nil, false, nil }

// AddComment likewise: unhandled, so the hub keeps the comment itself.
func (ForeignSource) AddComment(string, string, string) (bool, error) { return false, nil }
