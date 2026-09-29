// package: hub/flow/agent / layout
// type:    assembly (each role's default drawing, collected)
// job:     hand the debug view each role's checked-in layout, and name the file a developer's export
// of it is pasted into.
// limits:  the registry. The positions are each role package's own layout.go; producing them is
// the debug view's export (its URL: `sindri hub info`; open it with ?dev).
package agent

import (
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/coauthor"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/planner"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/reviewer"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

var layouts = map[string]machine.Layout{
	"worker":   worker.Layout,
	"planner":  planner.Layout,
	"reviewer": reviewer.Layout,
	"coauthor": coauthor.Layout,
}

// LayoutOf is one role's default drawing; empty until a developer exports one.
func LayoutOf(role string) machine.Layout { return layouts[role] }

// LayoutFile is the repo-relative file holding a role's layout, and the package it declares — what
// the export says to replace.
func LayoutFile(role string) (path, pkg string) {
	return "internal/hub/flow/agent/roles/" + role + "/layout.go", role
}
