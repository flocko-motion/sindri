// package: hub/flowtest / reviews
// type:    assembly (a review row, as the hub would file it)
// job:     register a reviewer and file the unclaimed review row a pull request gets when nobody is
// reading it — the two facts, and nothing that hands one to the other.
// limits:  the facts. WHO holds a review is the reviewer's own map's to decide, so a test that needs
// a hold runs the machine (-> Engine.Look) rather than writing one here.
package flowtest

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// Reviewer puts one reviewer on a project's roster, with the workspace its checkout goes into.
func Reviewer(t *testing.T, ps *store.ProjectStore, name string) {
	t.Helper()
	if err := ps.PutAgent(store.Agent{Name: name, Role: "reviewer", Workspace: ".worktrees/" + name}); err != nil {
		t.Fatalf("put agent %s: %v", name, err)
	}
}

// FileReview files an UNCLAIMED review row on pr, exactly as a pull request filed with nobody
// reading it gets one. It assigns nobody: a fixture that wrote the hold itself could build a world
// the machine would never produce — a reviewer holding an assigned review with its pod down — and
// then assert that it behaves correctly, which is how one was stranded.
func FileReview(t *testing.T, ps *store.ProjectStore, pr string) int64 {
	t.Helper()
	id, err := ps.AddReview(pr, "review it")
	if err != nil {
		t.Fatalf("add review on %s: %v", pr, err)
	}
	return id
}
