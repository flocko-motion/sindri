// package: hub / names
// type:    headless helper
// job:     auto-name new agents after Norse dwarves — friends of Sindri the
//
//	smith. The binary names "sindri" and "brokkr" are never handed out.
//	Names are picked at random and are unique across ALL projects, so each
//	dwarf identifies one agent machine-wide (recognisable on the unified
//	cross-repo board).
//
// limits:  only allocates names; registering and launching the agent are the
//
//	hub's (-> hub.go).
package hub

import (
	"fmt"
	"math/rand"
)

// dwarfNames are Norse dwarves (Dvergatal + the smith Eitri), lowercased for use
// as agent/workspace names. Deliberately excludes "sindri" and "brokkr" — those
// are the two binaries (the orchestrator and its toolbelt).
var dwarfNames = []string{
	"eitri", "dvalin", "durin", "dain", "nain", "fjalar", "galar",
	"alviss", "andvari", "regin", "bifur", "bofur", "bombur", "dori", "nori",
	"ori", "fili", "kili", "gloin", "oin", "balin", "dwalin", "thorin",
	"thrain", "fundin", "nyi", "nidi", "nordri", "sudri", "austri", "vestri",
	"frar", "loni", "jari", "hepti", "nar", "lit",
}

// autoName returns a random dwarf name unused by any agent across all projects, so
// names are globally unique and don't always start at "eitri". Once the pool is
// exhausted it appends a numeric suffix (thorin2, …) so creation never fails.
func (h *Hub) autoName() (string, error) {
	agents, err := h.store.AllAgents()
	if err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(agents))
	for _, a := range agents {
		taken[a.Name] = true
	}
	for _, i := range rand.Perm(len(dwarfNames)) { // random order → variety
		if !taken[dwarfNames[i]] {
			return dwarfNames[i], nil
		}
	}
	for i := 2; ; i++ {
		for _, j := range rand.Perm(len(dwarfNames)) {
			if cand := fmt.Sprintf("%s%d", dwarfNames[j], i); !taken[cand] {
				return cand, nil
			}
		}
	}
}
